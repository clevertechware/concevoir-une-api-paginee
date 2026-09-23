# L'implémentation Go

L'API paginée de [l'article](https://www.clevertechware.fr/blog/posts/2026/concevoir-une-api-paginee),
en Go 1.26. Elle implémente [`../spec/contract.md`](../spec/contract.md), comme
[`../spring-boot/`](../spring-boot) : un curseur émis ici est accepté là-bas.

Chaque affirmation de l'article a un test qui interroge un vrai PostgreSQL et lit
son plan d'exécution. Sur le jeu de test à 100 000 lignes, la même page à 99 800
lignes de profondeur coûte **14 blocs en keyset et 3 434 en `OFFSET`**.

## Démarrage

Le schéma et les données sont de l'infrastructure partagée, montée depuis la racine :

```bash
cd .. && make db-up && make seed ROWS=100000
```

Puis, ici :

```bash
make run                    # http://localhost:8080
```

```bash
curl 'http://localhost:8080/v1/transactions?account_id=42&limit=3'
```

Rejouez le `page.next` de la réponse sur l'API Spring Boot (port 8081) : la
pagination continue sans rien remarquer.

## Ce que le projet expose

| Endpoint | Ce qu'il démontre |
|---|---|
| `GET /v1/transactions` | La pagination keyset recommandée : curseur opaque signé, coût indépendant de la profondeur |
| `GET /v1/transactions/offset` | Le contre-exemple, pour être mesuré à côté — pas pour être réutilisé |
| `GET /v1/transactions/count-estimate` | La réponse honnête à « donnez-moi un total » : une estimation, assumée comme telle |
| `GET /healthz` | L'état du pool |

## Ce qu'il faut lire

- **[`pkg/cursor/cursor.go`](pkg/cursor/cursor.go)** : le curseur, public car
  c'est une frontière de contrat. La charge utile s'écrit **à la main** :
  `json.Marshal` changerait les octets (ordre des clés, RFC3339Nano), donc la
  signature, donc casserait l'interopérabilité avec Java.
- **[`internal/postgres/queries.go`](internal/postgres/queries.go)** : les huit
  requêtes, écrites en entier : quatre variantes (première page / suivantes ×
  filtrée / non filtrée) par sens de tri. La requête unique `($1 IS NULL OR …)`
  s'effondre dès que le driver bascule sur un plan générique.
- **[`internal/service/transactions.go`](internal/service/transactions.go)** : le
  `limit + 1` et la ligne jetée, soit `has_more` pour le prix d'une ligne, contre
  101 ms pour un `COUNT(*)`.

## Tester

```bash
make test-unit          # rapide, sans Docker
make test-integration   # testcontainers, un conteneur pour le paquet
make explain            # uniquement les mesures de plan, avec leurs chiffres
```

Les tests d'intégration montent leur propre PostgreSQL, y appliquent
`../sql/01-schema.sql` et sèment leurs données : pas besoin du `make seed`.

### Ce que chaque test prouve

| Affirmation de l'article | Le test |
|---|---|
| Le keyset ne dérive pas | `TestKeysetWalk_DoesNotDrift` — on parcourt 200 lignes en insérant 3 lignes en tête entre chaque page : aucun doublon, aucune ligne sautée |
| …et l'`OFFSET`, si | `TestOffsetWalk_Drifts` — le même parcours rend 27 doublons et perd 27 lignes, **sans lever la moindre erreur** |
| Le coût ne dépend pas de la profondeur | `TestExplain_KeysetCostDoesNotGrowWithDepth` — 12 blocs près du haut, 14 blocs 99 800 lignes plus bas |
| …contrairement à l'`OFFSET` | `TestExplain_OffsetCostGrowsWithDepth` — 8 blocs à la profondeur 0, 3 434 à la profondeur 99 900 |
| La borne doit être une `Index Cond` | `TestExplain_NextPageBoundIsAnIndexCond` — sur les trois variantes, la borne positionne le scan et aucune ligne n'est lue pour être jetée |
| Une seule requête pour toutes les pages est un piège | `TestExplain_TheSingleQueryTrap` — le plan *custom* simplifie le `IS NULL` et paraît sain ; le plan *générique* montre `Rows Removed by Filter: 999` |
| Le tie-breaker n'est pas optionnel | `TestKeysetWalk_NeedsTheTieBreakerOnIdenticalTimestamps` — 50 lignes au même `created_at`, parcours complet et sans doublon |
| Le curseur est infalsifiable | `TestDecode_RejectsAnythingButAnUntouchedToken` et `TestList_RejectsACursorThatDoesNotBelongToTheRequest` — un octet modifié part en `400 invalid_cursor` |
| Le curseur est lié à ses filtres | Le même test — un token émis pour `account_id=42` rejoué sur `43`, sur un autre `status` ou à l'envers part en `400 cursor_filter_mismatch` |
| Un curseur périmé n'est pas une erreur de syntaxe | `TestList_ExpiresACursorPastItsTTL` + `TestList_MapsCursorFailuresToTheContractedStatus` — au-delà de 72 h, `410 cursor_expired` |
| Le curseur est interopérable | `TestConformance_MatchesTheSharedVectors` — les trois vecteurs de `../spec/cursor-vectors.json` sont reproduits **à l'octet près**, empreinte, charge utile et token |
| `has_more` ne coûte qu'une ligne | `TestList_AsksForOneRowMoreThanThePageAndDropsIt` — le repository reçoit `limit + 1`, le client reçoit `limit` |
| `limit` se plafonne, il ne se rejette pas | `TestList_CapsTheLimitInsteadOfRejectingIt` — `limit=10000` renvoie 100 lignes et un `200` |
| Pas de `total`, jamais | `TestCountEstimate_ApproximatesTheTableWithoutCountingIt` — `reltuples` plutôt qu'un `COUNT(*)`, et `"exact": false` dans la réponse |

## Configuration

`application.yaml`, surchargeable par l'environnement (préfixe `PAGINATION_`,
`__` pour la profondeur) :

```bash
PAGINATION_CURSOR__KEY=une-vraie-cle PAGINATION_POSTGRES__HOST=db make run
```

La clé de signature committée est une clé de développement. Une clé vide fait
refuser le démarrage : un curseur non signé est forgeable.

## Structure

```
main.go                  câblage explicite, signal.NotifyContext, arrêt gracieux
internal/
  config/                koanf : application.yaml puis l'environnement
  domain/                entités, tri, bornes de pagination, erreurs de validation
  handler/               gin, les trois endpoints, le mapping erreur → statut
  postgres/              le repository et ses huit requêtes, en lecture seule
  service/               le curseur, le limit + 1, l'aiguillage première/suivante
  testutil/              le conteneur PostgreSQL et les jeux de données de test
pkg/
  cursor/                le format du curseur — public, c'est le contrat
  logger/                slog derrière une interface, pour un logger muet en test
```
