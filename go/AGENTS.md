# AGENTS.md

Contexte pour les agents travaillant sur `go/`.

## Ce qu'est ce projet

L'implémentation Go du contrat de `../spec/contract.md`, compagnon de l'article
« Concevoir une API paginée qui tient la charge ». Ce n'est pas une application à
faire grandir : **c'est une démonstration**. Chaque endpoint illustre une
affirmation précise de l'article, et chaque affirmation a un test qui la prouve
contre un vrai PostgreSQL.

Avant d'ajouter quoi que ce soit : *quelle thèse de l'article cela sert-il ?* Si
la réponse est « aucune », c'est hors périmètre.

`../spring-boot/` implémente le même contrat. Les deux doivent rester
interchangeables du point de vue du client : un curseur émis ici est accepté
là-bas, et `../spec/cursor-vectors.json` le vérifie des deux côtés.

## Stack

Go 1.26, gin, pgx/v5 (+pgxpool), koanf, slog via `pkg/logger`, testify +
testcontainers-go. PostgreSQL 18.

Pas de golang-migrate : le schéma (`../sql/01-schema.sql`) est de l'infra
partagée, appliquée par le conteneur. **L'application est en lecture seule.** Les
tests d'intégration lisent ce même fichier et l'appliquent dans leur propre
conteneur.

## Architecture

```
handler → service → domain
              ↓
          postgres (repository) → domain
```

Interfaces déclarées **côté consommateur**, en minuscule :
`internal/service/ports.go` pour le repository, `internal/handler/http_transaction.go`
pour son service. Handlers HTTP : `HTTP*Handler` dans `http_*.go`.

`pkg/cursor` est le seul paquet public, et c'est délibéré : le curseur est une
**frontière de contrat**, pas un détail d'implémentation. Il ne dépend d'aucun
paquet interne.

## Les trois règles à ne pas casser

**1. Deux requêtes, jamais une.** `FirstPage` et `NextPage` sont deux méthodes,
et les huit constantes SQL de `internal/postgres/queries.go` sont écrites en
entier. La tentation d'unifier avec `($1 IS NULL OR …)` annule silencieusement
tout le bénéfice dès que pgx bascule sur un plan générique.
`TestExplain_TheSingleQueryTrap` mesure le dégât : 999 lignes lues puis jetées.

**2. La charge utile du curseur s'écrit à la main.** `json.Marshal` ordonnerait
les clés selon la struct et formaterait le timestamp en RFC3339Nano, qui tronque
les zéros de fin. L'un ou l'autre change les octets, et changer les octets change
la signature. Voir `Cursor.canonicalPayload`.

**3. `LIMIT limit+1`, et la ligne excédentaire ne sort jamais.** Le `+1` vit dans
le service (`Transactions.List`), pas dans le repository : c'est là que la
décision `has_more` se prend, donc c'est là qu'on doit la voir.

## Tests

```bash
make test-unit          # rapide, sans Docker (-short saute les conteneurs)
make test-integration   # testcontainers
make explain            # uniquement les mesures de plan, avec leurs chiffres
make mocks              # régénère les doubles après un changement de port
```

- Unitaires : table-driven, `t.Context()`, `logger.NewNoOpLogger()`. Les doubles
  sont **générés par mockery** (`go tool mockery`, configuré dans `.mockery.yml`)
  et vivent dans un sous-paquet `mocks/` à côté du port qu'ils implémentent :
  `internal/service/mocks`, `internal/handler/mocks`. Ils sont commités ; après
  avoir touché à une interface, lancer `make mocks`.
- Les attentes portent sur les arguments (`EXPECT().FirstPage(mock.Anything,
  query, limit+1)`) plutôt que sur des champs capturés, et un port construit
  avec `mocks.NewX(t)` **échoue le test sur un appel non attendu**. C'est ce qui
  prouve, sans assertion supplémentaire, qu'une requête rejetée en 400 n'atteint
  jamais le service, et qu'une première page n'appelle jamais `NextPage`.
- Intégration : un conteneur par package via `testutil.RunWithPostgres` dans
  `TestMain`. L'application ne fait que lire, donc `RepositorySuite` sème une
  fois dans `SetupSuite` et ne s'isole pas. Les tests qui écrivent (`walk_test.go`)
  **sèment eux-mêmes** ce dont ils ont besoin, pour que l'ordre d'exécution
  n'ait aucune importance.
- `internal/testutil/seed.go` n'est **pas** `../sql/seed.sql` : il concentre les
  lignes sur 50 comptes, sinon aucune partition de tenant n'est assez profonde
  pour que le piège de la requête unique fasse des dégâts visibles.

## Voir les requêtes

`logging.level: debug` (ou `PAGINATION_LOGGING__LEVEL=debug`) installe le
traceur pgx et journalise chaque énoncé SQL, ses arguments et sa durée. Il n'y a
pas de réglage dédié : le niveau de log est l'interrupteur, et la décision se
prend une seule fois dans `postgres.NewPool`, parce que pgx alloue à chaque
requête dès qu'un traceur existe. C'est ce qui rend visible la règle des deux
requêtes sans lire le code — et cela journalise les arguments, donc les filtres
du client.

La vue serveur existe en parallèle : `make db-up PG_LOG_STATEMENT=all` puis
`make db-logs`, à la racine du dépôt. Elle voit tout ce qui atteint la base, y
compris le seed et `make bench`, mais ne sait pas quelle requête HTTP l'a
provoqué. Éteinte par défaut, sinon `make bench` mesurerait ses propres écritures.

**Piège récurrent** : les compteurs de blocs varient selon ce qui est déjà en
cache. Toute comparaison de profondeur passe par `RepositorySuite.measure`, qui
chauffe le cache avant de mesurer.

**Autre piège** : dans un `defer` ou un `t.Cleanup`, `t.Context()` est déjà
annulé. Utiliser `context.WithoutCancel(ctx)`, sinon le nettoyage ne part jamais.

## Conventions

- Commentaires de code en anglais. README et ce fichier en français.
- Les commentaires de `internal/postgres/queries.go` expliquent le *pourquoi* :
  ce sont eux que le lecteur de l'article vient lire. Ne pas les raccourcir.
- Paramètres de requête : un struct `xxxRequest` par endpoint dans
  `internal/handler/requests.go`, lié par `c.ShouldBindQuery`. Chaque paramètre
  est un type nommé qui se valide lui-même dans `UnmarshalParam`
  (`binding.BindUnmarshaler`) plutôt que dans un tag `binding` : le validateur
  ne s'exécute qu'après la conversion, donc un `limit=abc` échoue avant lui avec
  une erreur qui ne nomme pas le champ, alors que le contrat répond avec le code
  du paramètre fautif.
- Frontière du service : un `xxxParams` du domaine par appel
  (`domain.ListParams`, `domain.OffsetParams`) — `Params` côté domaine,
  `Request` côté HTTP, pour que les deux ne se confondent pas. `Limit` et
  `Cursor` restent **hors** de `ListQuery` : `ListQuery` est ce que couvre
  l'empreinte du curseur et ce que reçoit le repository, or un `limit` peut
  changer en cours de parcours sans invalider le curseur, et le repository ne
  voit jamais de jeton.
- Erreurs : sentinelles de validation dans `internal/domain`, sentinelles de
  curseur dans `pkg/cursor` (qui ne peut pas dépendre d'`internal/`). Le handler
  mappe les deux familles au même endroit, `internal/handler/errors.go`. Un 500
  ne renvoie jamais la chaîne d'erreur interne.
- Les bornes de pagination (20/100, 1000/5000) sont des constantes de
  `internal/domain`, pas de la configuration : elles font partie du contrat, et
  un déploiement qui pourrait les changer rendrait le contrat faux.
