# Pagination keyset — implémentation Spring Boot

L'une des deux implémentations du contrat décrit dans [`../spec/contract.md`](../spec/contract.md),
compagnon de l'article [« Concevoir une API paginée qui tient la charge »](https://www.clevertechware.fr/blog/posts/2026/concevoir-une-api-paginee).

L'autre vit dans [`../go/`](../go). Les deux parlent le même curseur : un token émis ici est accepté
là-bas, et réciproquement. C'est toute la thèse de l'article — le curseur opaque est une **frontière
de contrat**, pas un détail d'implémentation.

## Ce que ce projet démontre

Ce n'est pas une application à faire grandir, c'est une démonstration outillée. Chaque affirmation
de l'article a ici un test qui la prouve contre un vrai PostgreSQL.

- **Deux requêtes SQL distinctes**, une pour la première page, une pour les suivantes. Jamais de
  `(? IS NULL OR …)` : c'est le piège central de l'article, et un test lit le plan d'exécution pour
  montrer qu'il se déclenche vraiment.
- **Un curseur opaque signé**, sérialisé en JSON canonique octet pour octet, vérifié contre les
  vecteurs de conformance partagés avec l'implémentation Go.
- **`LIMIT n+1`** pour répondre à `has_more` sans jamais compter, et la ligne excédentaire ne sort
  jamais du serveur.
- **Aucun `total`** dans la réponse. Un endpoint séparé rend une estimation, et dit que c'en est une.
- **Pas de JPA.** `JdbcClient` et du SQL écrit à la main : le sujet de l'article, c'est le SQL, il
  doit rester lisible.

## Stack

Java 21, Spring Boot 3.5, Maven (wrapper fourni), `spring-boot-starter-web`,
`spring-boot-starter-jdbc` avec `JdbcClient`, driver PostgreSQL, HikariCP.
Tests : JUnit 5, AssertJ, Mockito, Testcontainers.

Le pool est configuré en lecture seule : le schéma et les données sont de l'**infrastructure
partagée**, appliquée par le conteneur de la racine et par `make seed`. L'application ne migre rien,
ne crée rien — d'où l'absence de Flyway.

## Lancer

Depuis la racine du dépôt, une base est nécessaire :

```bash
make db-up                  # PostgreSQL + le schéma
make seed ROWS=100000       # de quoi boucler vite (ROWS=10000000 pour le dataset de l'article)
```

Puis :

```bash
make run                    # http://localhost:8081
```

`spring-boot:run` lit `src/main/resources/application.yaml` : port **8081**,
`localhost:5432/pagination`, clé HMAC de démo et TTL de curseur à 72 h.
L'API Go écoute sur `:8080`, ce qui permet de rejouer un curseur d'une API sur l'autre :

```bash
NEXT=$(curl -s 'http://localhost:8080/v1/transactions?account_id=42&limit=3' | jq -r .page.next)
curl -s "http://localhost:8081/v1/transactions?account_id=42&limit=3&cursor=$NEXT" | jq
```

## Tester

```bash
make test-unit          # rapide, aucun Docker requis
make test-integration   # Testcontainers, Docker requis
make test               # les deux
```

Les tests d'intégration démarrent **un seul** conteneur PostgreSQL 18.4 pour toute la suite, y
appliquent `../sql/01-schema.sql` — le fichier même que la stack compose applique, sans quoi les
assertions sur les plans ne vaudraient rien — puis un jeu de 100 000 lignes défini dans
`src/test/resources/sql/test-seed.sql`.

La séparation passe par le tag JUnit `integration`, porté par la classe de base
`AbstractDatabaseTest`. `./mvnw test` reste donc utilisable sur une machine sans Docker.

## Ce que chaque test prouve

### `CursorConformanceVectorsTest` — le curseur est interopérable

Pour chacun des trois vecteurs de `../spec/cursor-vectors.json` : l'empreinte est recalculée depuis
les filtres, la charge utile canonique est reproduite **octet pour octet**, le token signé aussi, et
le décodage retrouve la position de départ. C'est ce test qui garantit qu'un curseur émis par l'API
Go passe ici sans rien changer.

### `CursorCodecTest` — le curseur est infalsifiable

Un octet modifié, une signature d'une autre clé, un base64 invalide, un token trop court, une
version de format inconnue : tout part en `invalid_cursor`. Un curseur émis il y a plus de 72 h part
en `cursor_expired`, distinct du précédent — la demande était bien formée, c'est la position qui
n'existe plus. La précision microseconde de `created_at` survit à l'aller-retour.

### `QueryParametersTest` — `limit` se plafonne, il ne se rejette pas

`limit=10000` devient 100. `limit=-1`, `limit=abc` deviennent `400 invalid_limit`. Idem pour `page`,
`sort` et `account_id`, chacun avec le code que le contrat lui donne.

### `TransactionServiceTest` — `has_more` ne coûte qu'une ligne

Le service demande `limit + 1` à la base (vérifié par un `ArgumentCaptor`), n'en rend que `limit`,
et fabrique `next` à partir de la dernière ligne réellement rendue. Il refuse un curseur rejoué sur
un autre compte, avec un autre `status`, ou avec le tri inversé. Aucune base n'est nécessaire ici.

### `ExecutionPlanIT` — le coût ne dépend pas de la profondeur

- `placesTheCursorBoundUnderIndexCond…` : le plan de la requête « pages suivantes » montre la borne
  du curseur sous `Index Cond`, sur `idx_txn_acct` pour la variante filtrée et sur
  `idx_txn_created_id` pour l'autre, et ne contient aucun `Rows Removed by Filter`.
- `readsTheSameNumberOfBlocksWhateverTheDepthOfTheCursor` : le nombre de blocs lus à la profondeur 10
  et à la profondeur 1 900 est le même à deux blocs près.
- `offsetPaginationReadsMoreAndMoreBlocks…` : le contre-exemple, `OFFSET 90000` lit au moins cinq
  fois plus de blocs que `OFFSET 0`.
- `theSingleStatementGuardedByIsNullDegradesIntoAFilter…` : la requête unique
  `($1 IS NULL OR (created_at, id) < ($1, $2))`, préparée puis exécutée sous
  `plan_cache_mode = force_generic_plan`, montre `Filter:` et `Rows Removed by Filter`. C'est le
  piège, exécuté plutôt que raconté.
- `theTwoStatementVariantKeepsItsIndexBound…` : la vraie requête de production, préparée de la même
  façon, sous le même plan générique, à la même profondeur, garde son `Index Cond`. Le contraste
  entre ces deux tests est l'argument.

Les plans sont lus sur la **chaîne SQL de production** : `TransactionQueries` est délibérément
visible depuis le test, et les paramètres sont positionnels pour qu'un `EXPLAIN` puisse la reprendre
telle quelle.

### `KeysetDriftIT` — le keyset ne dérive pas

- Un parcours keyset complet, interrompu après la première page par cinq insertions **en tête** :
  aucun doublon, et les soixante lignes d'origine sont toutes rendues.
- Le même parcours en `OFFSET`, avec les mêmes insertions : des lignes déjà rendues reviennent.
- Le même parcours en `OFFSET` avec des suppressions en tête : des lignes ne sont **jamais** rendues,
  sans qu'aucune erreur ne soit levée.
- Cinquante lignes partageant un `created_at` identique : le parcours reste complet et sans doublon,
  parce que `id` est dans la clé de tri. C'est le tie-breaker.
- Le parcours ascendant visite exactement les mêmes lignes que le descendant.

### `TransactionApiIT` — le contrat HTTP

L'enveloppe, `id` sérialisé en chaîne, `created_at` en RFC 3339 `Z`, l'absence de `total`, `next` à
`null` en fin de parcours, le plafonnement de `limit` qui répond `200`, la continuation d'un
parcours depuis le curseur rendu, et les quatre codes d'erreur du contrat : `400 invalid_cursor`,
`400 cursor_filter_mismatch`, `410 cursor_expired`, `400 invalid_limit` / `invalid_sort` /
`invalid_account_id` / `invalid_page`. Plus `/v1/transactions/offset`, `/v1/transactions/export`,
`/v1/transactions/count-estimate` et `/healthz`.

## Architecture

```
web (TransactionController, ApiExceptionHandler, DTO)
 └─ service (TransactionService)
     ├─ cursor (CursorCodec, FilterFingerprint)
     └─ repository (TransactionRepository, TransactionQueries) → PostgreSQL
```

- `TransactionQueries` regroupe **toutes** les requêtes, en clair, avec les quatre variantes keyset
  (première page / pages suivantes × filtrée / non filtrée).
- `TransactionRepository` expose une méthode par requête. Pas de query builder : quelle variante
  s'exécute est précisément ce qui décide du plan.
- `CursorCodec` écrit la charge utile canonique à la main (`StringBuilder`) et ne s'en remet à
  Jackson qu'en **lecture** : ni l'ordre des clés ni le format temporel d'un sérialiseur générique
  ne font partie de son contrat, alors qu'ils font partie du nôtre.
- `ApiExceptionHandler` traduit les exceptions métier vers l'enveloppe d'erreur. Un `500` journalise
  et ne rend jamais le message interne.

## Précisions par rapport au contrat

Quelques points que `../spec/contract.md` laisse ouverts, tranchés ici :

- `limit=0` et `size=0` sont refusés en `400 invalid_limit` : le contrat ne parle que du négatif et
  du non numérique, mais une page de zéro ligne ne veut rien dire dans un parcours.
- `size` invalide sur `/v1/transactions/offset` renvoie `invalid_limit`, le contrat ne nommant pas
  de code propre à ce paramètre.
- `after_id` invalide renvoie `invalid_after_id`, code absent du tableau du contrat.
- `account_id` doit être strictement positif ; `0` est la valeur réservée à « pas de filtre » dans
  l'empreinte.
- `status` est accepté et entre dans l'empreinte des filtres, mais n'atteint pas encore le SQL,
  comme le contrat le prévoit. Le mettre dans l'empreinte dès maintenant évite qu'un curseur émis
  avant son implémentation soit accepté après.
- `/healthz` renvoie `{"status":"down"}` avec un `503` quand le pool ne répond pas ; le contrat ne
  fixe que le statut.

## Configuration

| Propriété | Défaut | Rôle |
|---|---|---|
| `server.port` | `8081` | l'API Go prend `8080`, les deux tournent côte à côte |
| `spring.datasource.url` | `jdbc:postgresql://localhost:5432/pagination` | la base de `compose.yaml` |
| `pagination.cursor.key` | `concevoir-une-api-paginee-dev-key` | clé HMAC. En production elle vient de la configuration ; la changer invalide tous les curseurs en circulation |
| `pagination.cursor.time-to-live` | `72h` | au-delà, `410 cursor_expired` |
