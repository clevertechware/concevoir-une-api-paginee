# AGENTS.md — spring-boot/

## Ce qu'est ce projet

L'implémentation Java du contrat d'API décrit dans `../spec/contract.md`, compagnon de l'article
« Concevoir une API paginée qui tient la charge ». C'est une **démonstration**, pas un produit :
chaque endpoint illustre une affirmation de l'article et chaque affirmation a un test qui la prouve.
Le code est destiné à être *lu* par les lecteurs de l'article.

Un jumeau Go vit dans `../go/`. Les deux doivent rester interchangeables du point de vue du client.

## Stack

Java 21, Spring Boot 3.5, Maven (wrapper `./mvnw`), `JdbcClient` (pas de JPA — on veut voir le SQL),
PostgreSQL, HikariCP. Tests : JUnit 5, AssertJ, Mockito, Testcontainers.

## Architecture

```
web (controller, DTO, @RestControllerAdvice) → service → domain
                                                  ↓
                              repository (JdbcClient) + TransactionQueries → PostgreSQL
```

Le curseur a son propre package `fr.clevertechware.pagination.cursor` : `CursorCodec`, `Cursor`,
`FilterFingerprint`, ses deux exceptions.

## Règles à ne pas casser

1. **Deux requêtes SQL distinctes**, première page et pages suivantes, jamais fusionnées par un
   `(? IS NULL OR …)`. C'est le piège central de l'article. `ExecutionPlanIT` échouera si on le
   fusionne, et c'est voulu.
2. **La charge utile du curseur est écrite à la main.** Ordre des clés figé, aucun espace,
   `created_at` en RFC 3339 UTC avec exactement six décimales. Ne jamais confier la **production**
   de cette chaîne à Jackson : ni l'ordre des champs ni le format temporel ne sont garantis. En
   lecture, Jackson convient. `CursorConformanceVectorsTest` échouera à l'octet près sinon, et
   l'interopérabilité avec l'implémentation Go avec lui.
3. **Placeholders positionnels (`?`) dans `TransactionQueries`.** Les tests de plan reprennent la
   chaîne SQL de production telle quelle et la passent à `EXPLAIN`. Passer aux paramètres nommés
   casserait cette propriété.
4. **`LIMIT limit + 1`**, et la ligne excédentaire est coupée dans le service. Elle ne sort jamais.
5. **Pas de `total`** dans les réponses de liste. `count-estimate` existe pour ça et assume son
   `exact: false`.
6. **`id` sérialisé en chaîne** dans le JSON, `created_at` en RFC 3339 `Z`.
7. **Pas de migration.** Le schéma est de l'infra partagée (`../sql/01-schema.sql`), l'application
   est en lecture seule (`spring.datasource.hikari.read-only: true`). Pas de Flyway, pas de
   Liquibase, pas de `schema.sql`.
8. **Un `500` ne fuit jamais le message interne.** `ApiExceptionHandler` journalise et rend
   `{"error":{"code":"internal_error","message":"unexpected error"}}`.
9. Commentaires de code **en anglais**, `README.md` **en français**.

## Pièges de test

- **La forme du jeu de données compte autant que sa taille.** `src/test/resources/sql/test-seed.sql`
  donne 2 % de la table au compte 42. Répartir les lignes équitablement suffit à faire abandonner
  `idx_txn_acct` au planificateur : le filtre tenant redevient un `Filter` et les assertions de plan
  mesurent alors le mauvais index. Ne pas « simplifier » cette distribution.
- Les tests d'intégration portent `@Tag("integration")` via `AbstractDatabaseTest`. `./mvnw test`
  doit rester exécutable sans Docker ; `-Pintegration-only` ne lance que les tests Docker et
  `-Pintegration` lance tout.
- Un seul conteneur pour toute la suite, démarré dans le bloc statique de `AbstractDatabaseTest`.
  Ne pas le passer en `@Container` : JUnit l'arrêterait après chaque classe.
- Le pool applicatif est en lecture seule. Un test qui écrit ouvre sa propre connexion via
  `openConnection()`.
- `SET plan_cache_mode`, `PREPARE` et `EXECUTE` sont des états de **session** : ces tests ouvrent une
  connexion JDBC brute plutôt que de passer par le pool.
- `KeysetDriftIT` écrit dans un compte dédié (900) et le nettoie en `@AfterEach`. Le conteneur est
  partagé : tout test qui écrit doit se cantonner à ses propres comptes et nettoyer derrière lui.

## Commandes

```bash
make test-unit           # rapide, sans Docker
make test-integration    # Testcontainers
make test                # les deux
make run                 # :8081, base sur localhost:5432
./mvnw -Pintegration clean package
```
