# Concevoir une API paginée qui tient la charge

Le compagnon de code de [l'article du même nom](https://www.clevertechware.fr/blog/posts/2026/concevoir-une-api-paginee).

`OFFSET 5 000 000` : 321 ms et 65 890 blocs lus pour rendre 20 lignes. Le même
résultat en keyset : 0,009 ms et 4 blocs. Ce dépôt reproduit la mesure, puis
implémente **deux fois** le contrat d'API que l'article recommande — une fois en
Go, une fois en Spring Boot.

Ce n'est pas une application à faire grandir : c'est une démonstration. Chaque
endpoint existe pour illustrer une affirmation précise de l'article, et chaque
affirmation a un test qui la prouve contre un vrai PostgreSQL.

## Pourquoi deux implémentations

Parce que la thèse centrale de l'article est que le curseur est une **frontière de
contrat**, pas un détail d'implémentation. Deux langages, deux stacks, un seul
format de curseur : un token émis par l'API Go est accepté tel quel par l'API
Spring Boot. `spec/cursor-vectors.json` fige cette compatibilité et les deux
suites de tests la vérifient.

Si vous ne deviez lire qu'un fichier, lisez [`spec/contract.md`](spec/contract.md) :
c'est le contrat que les deux projets implémentent.

## Structure

```
.
├── compose.yaml            PostgreSQL 18.4, partagé par les deux projets
├── sql/
│   ├── 01-schema.sql       la table et ses deux index
│   ├── seed.sql            le jeu de données de l'article, graine fixée
│   └── benchmark.sql       les EXPLAIN ANALYZE, dans l'ordre de l'article
├── spec/
│   ├── contract.md         le contrat HTTP + le format du curseur (normatif)
│   └── cursor-vectors.json les vecteurs de conformance inter-langages
├── go/                     Go 1.26, gin, pgx/v5, koanf, testcontainers
└── spring-boot/            Java 21, Spring Boot 3, JdbcClient, Testcontainers
```

Le schéma et les données sont de l'**infrastructure partagée** : ils sont
appliqués par le conteneur et par `make seed`, jamais par les applications. Les
deux API sont en lecture seule sur la même base, et écoutent sur des ports
distincts (Go `:8080`, Spring Boot `:8081`), ce qui permet de les interroger
côte à côte avec le même curseur.

## Démarrer

```bash
make db-up                  # PostgreSQL + le schéma
make seed                   # 10 000 000 de lignes (long) …
make seed ROWS=100000       # … ou de quoi boucler vite
make bench                  # rejoue les mesures de l'article
```

### Ce que `make bench` donne

Relevé sur un Mac de développement, PostgreSQL 18.4 en conteneur, `shared_buffers`
à ses 128 Mo par défaut, 10 000 000 de lignes. Les temps absolus dépendent de la
machine ; les rapports entre profondeurs, non.

| Requête | Lignes remontées sous le `Limit` | Blocs lus | Temps |
|---|---:|---:|---:|
| `LIMIT 20 OFFSET 0` | 20 | 4 | 0,034 ms |
| `LIMIT 20 OFFSET 500 000` | 500 020 | 8 398 | 37,7 ms |
| `LIMIT 20 OFFSET 5 000 000` | 5 000 020 | 83 952 | 369 ms |
| keyset, profondeur 500 000 | 20 | 5 | 0,030 ms |
| keyset, profondeur 5 000 000 | 20 | 5 | 0,029 ms |
| `COUNT(*)` | 10 000 000 | 99 405 | 111 ms |

Le `OFFSET` à 5 000 000 affiche aussi un `written` non nul : la requête ne modifie
rien, mais elle évince tellement de pages du cache qu'il faut en écrire sur disque
pour lui faire de la place. Une pagination profonde ne se contente pas d'être
lente pour celui qui la lance, elle vide le cache des autres.

Et le piège de la requête unique, en plan générique :

```text
Index Scan using idx_txn_acct on transactions (actual rows=20.00 loops=1)
  Index Cond: (account_id = 42)
  Filter: (($1 IS NULL) OR (ROW(created_at, id) < ROW($1, $2)))
  Rows Removed by Filter: 2451
```

La borne est passée sous `Filter`. On a réinventé l'`OFFSET`, avec la syntaxe du
keyset en prime. C'est pour ça que les deux implémentations émettent deux
requêtes distinctes.

Puis, au choix :

```bash
make go-run                 # http://localhost:8080
make java-run               # http://localhost:8081
```

```bash
curl 'http://localhost:8080/v1/transactions?account_id=42&limit=3'
```

Récupérez le `page.next` de la réponse et rejouez-le **sur l'autre API** : la
pagination continue sans rien remarquer.

```bash
curl "http://localhost:8081/v1/transactions?account_id=42&limit=3&cursor=$NEXT"
```

Relevé sur les deux serveurs lancés côte à côte, `account_id=42`, `limit=3` :

```text
page 1  — Go   :8080   9996899, 9995724, 9994722
page 2  — Java :8081   9992087, 9984810, 9983604   (curseur émis par Go)
page 3  — Go   :8080   9981250, 9981087, 9978954   (curseur émis par Java)
```

Neuf lignes strictement décroissantes, sans trou ni doublon, alors que le
parcours a changé de langage deux fois. Les trois curseurs portent la même
empreinte de filtres `zJlP3yfvPv8` — celle du vecteur n°2 de
`spec/cursor-vectors.json`.

C'est ce que veut dire « le curseur est une frontière de contrat ».

## Tester

```bash
make test                   # les deux suites
make go-test
make java-test
```

Les tests d'intégration démarrent leur propre conteneur via Testcontainers avec
un petit jeu de données : ils n'ont pas besoin du `make seed` ci-dessus et ne
touchent pas à la base de développement.

## Ce que chaque implémentation doit prouver

Les deux projets ont, sous une forme idiomatique à leur stack, un test pour
chacun de ces points :

| Affirmation de l'article | Ce que le test montre |
|---|---|
| Le keyset ne dérive pas | On insère des lignes en tête pendant un parcours : aucun doublon, aucune ligne sautée. L'`OFFSET` équivalent, lui, en produit |
| Le coût ne dépend pas de la profondeur | Le plan de la page 1 et celui de la page 5 000 lisent le même nombre de blocs |
| La borne doit être une `Index Cond` | Le plan de la requête « pages suivantes » ne contient jamais `Filter` sur le curseur |
| Une seule requête pour toutes les pages est un piège | Le plan générique de la variante `IS NULL OR …` montre `Rows Removed by Filter` |
| Le tie-breaker n'est pas optionnel | Sur des `created_at` identiques, le parcours reste complet et sans doublon |
| Le curseur est infalsifiable | Un token dont on modifie un octet part en `400 invalid_cursor` |
| Le curseur est lié à ses filtres | Un token émis pour `account_id=42` rejoué sur `account_id=43` part en `400 cursor_filter_mismatch` |
| Le curseur est interopérable | Les vecteurs de `spec/cursor-vectors.json` sont reproduits à l'octet près |
| `has_more` ne coûte qu'une ligne | `LIMIT n+1`, et la ligne excédentaire ne sort jamais du serveur |
| `limit` se plafonne, il ne se rejette pas | `limit=10000` renvoie 100 lignes et un `200` |

## Licence

MIT.
