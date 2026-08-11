# Le contrat partagé

Ce document est **normatif**. Les deux implémentations (`go/` et `spring-boot/`) le
respectent à l'octet près : un curseur émis par l'une est accepté par l'autre, et
`spec/cursor-vectors.json` le prouve dans les deux suites de tests.

C'est le point de la démonstration : un curseur opaque est une **frontière de
contrat**, pas un détail d'implémentation. S'il l'est vraiment, on doit pouvoir
changer de langage derrière sans que le client s'en aperçoive.

---

## 1. Endpoints

### `GET /v1/transactions` — la pagination keyset

La liste recommandée par l'article. C'est celle qu'un connecteur consomme.

| Paramètre    | Obligatoire | Défaut | Règle |
|--------------|-------------|--------|-------|
| `account_id` | non         | —      | filtre tenant, entier positif |
| `status`     | non         | `""`   | réservé, entre dans l'empreinte des filtres |
| `sort`       | non         | `created_at:desc` | `created_at:desc` ou `created_at:asc` uniquement |
| `limit`      | non         | `20`   | **plafonné à 100**, on plafonne, on ne rejette pas. `0` = absent (AIP-158), négatif = `400` |
| `cursor`     | non         | —      | opaque, signé. Absent = première page |

Réponse `200` :

```json
{
  "data": [
    {
      "id": "9500000",
      "account_id": 42,
      "amount_cents": 212190,
      "label": "VIREMENT 09500000",
      "created_at": "2021-01-08T01:46:40Z"
    }
  ],
  "page": {
    "next": "XpQF28PoX3CTQl3hKrIqSdr-SCaQJpMk4jUxxT1-...",
    "has_more": true
  }
}
```

- `id` est sérialisé en **chaîne** : un `bigint` dépasse le `Number.MAX_SAFE_INTEGER`
  de JavaScript, et le client qui le relit en JSON perdrait des bits en silence.
- `created_at` est en RFC 3339 UTC, suffixe `Z`.
- Pas de `total`. Jamais.
- `next` est `null` en fin de parcours, et c'est **la seule autorité** sur la fin.
- `has_more` est redondant avec `next`, volontairement : le contrat se lit sans
  documentation.

### `GET /v1/transactions/offset` — le contre-exemple

Le même dataset, paginé par `page` / `size`, pour que la démo puisse mesurer les
deux côte à côte. Il existe pour être comparé, pas pour être recommandé.

| Paramètre    | Défaut | Règle |
|--------------|--------|-------|
| `account_id` | —      | idem |
| `page`       | `1`    | 1-based, `>= 1` |
| `size`       | `20`   | plafonné à 100 |

Réponse `200` :

```json
{
  "data": [ … ],
  "page": { "page": 1, "size": 20, "has_more": true }
}
```

Toujours pas de `total` : le `COUNT(*)` coûte 4 000 fois la page. Un endpoint
séparé l'expose, en estimation assumée (voir plus bas).

### `GET /v1/transactions/count-estimate` — le total, assumé comme une estimation

Lit `reltuples` dans `pg_class` plutôt que de compter. Renvoie
`{"estimate": 10000000, "exact": false}`. Sert à montrer ce qu'on répond quand un
client réclame vraiment un total.

### `GET /healthz`

`{"status":"ok"}` si le pool répond, `503` sinon.

---

## 2. Erreurs

Enveloppe commune :

```json
{ "error": { "code": "invalid_cursor", "message": "cursor signature does not verify" } }
```

| Situation | Statut | `code` |
|---|---|---|
| Curseur illisible, signature invalide, mauvaise version | `400` | `invalid_cursor` |
| Curseur émis il y a plus de 72 h | `410` | `cursor_expired` |
| Filtres modifiés en cours de parcours | `400` | `cursor_filter_mismatch` |
| `limit` / `size` négatif ou non numérique | `400` | `invalid_limit` |
| `page` < 1 ou non numérique | `400` | `invalid_page` |
| `sort` inconnu | `400` | `invalid_sort` |
| `account_id` non numérique, nul ou négatif | `400` | `invalid_account_id` |

Un `limit=0` **n'est pas une erreur** : l'AIP-158 le lit comme « pas de préférence »
et le serveur applique son défaut. Seule une valeur négative est un bug client. Un
`account_id=0` en revanche est refusé, parce que `0` est précisément la valeur que
l'empreinte des filtres utilise pour dire « pas de filtre » : l'accepter rendrait
deux requêtes différentes indiscernables dans le curseur.

Le `410` sur curseur expiré n'est pas un `400` : la demande était bien formée,
c'est la position qui n'existe plus. C'est exactement ce dont le client a besoin
pour décider de repartir du début.

Un `500` ne renvoie jamais la chaîne d'erreur interne.

---

## 3. Le curseur opaque

### 3.1 Charge utile

Un objet JSON, **sans espace**, dont l'ordre des clés est figé :

```
{"v":1,"c":"<created_at>","i":<id>,"d":<bool>,"f":"<fingerprint>","t":<issued_at>}
```

| Clé | Type | Sens |
|-----|------|------|
| `v` | entier | version du format, `1` aujourd'hui. Une autre valeur ⇒ `invalid_cursor` |
| `c` | chaîne | la valeur `created_at` de la dernière ligne rendue |
| `i` | entier | son `id`, le tie-breaker |
| `d` | booléen | `true` si le tri est descendant. Rejette un curseur rejoué à l'envers |
| `f` | chaîne | empreinte des filtres |
| `t` | entier | date d'émission, secondes Unix. Sert au `410` |

`c` est formaté en **RFC 3339 UTC avec exactement 6 décimales** :
`2021-05-03T19:32:40.000000Z`. Six chiffres, parce que c'est la précision d'un
`timestamptz` PostgreSQL : ni troncature, ni ambiguïté de format entre les deux
langages.

- Go : `t.UTC().Format("2006-01-02T15:04:05.000000Z")`
- Java : `DateTimeFormatter.ofPattern("uuuu-MM-dd'T'HH:mm:ss.SSSSSS'Z'")` sur un
  `OffsetDateTime` ramené en UTC.

Le JSON étant canonique, aucune des deux implémentations ne doit s'en remettre au
sérialiseur par défaut de son langage pour **produire** la charge utile : on
l'écrit à la main. En **lecture**, un parseur JSON normal convient.

### 3.2 Empreinte des filtres

```
fingerprint = base64url_sans_padding( SHA256("<account_id>|<status>|<sort>")[0:8] )
```

`account_id` vaut `0` quand le filtre est absent, `status` la chaîne vide, `sort`
la valeur normalisée (`created_at:desc` / `created_at:asc`). Le handler compare
l'empreinte du curseur reçu à celle de la requête courante ; si elles diffèrent,
c'est `400 cursor_filter_mismatch`, pas une page incohérente.

### 3.3 Signature

```
token = base64url_sans_padding( HMAC_SHA256(clé, charge_utile) || charge_utile )
```

Les 32 octets du HMAC d'abord, la charge utile ensuite. La comparaison de
signature se fait en **temps constant** (`hmac.Equal`, `MessageDigest.isEqual`).

Le contenu reste lisible pour qui décode le base64 : c'est voulu. On signe, on ne
chiffre pas. Chiffrer n'apporterait quelque chose que si la position elle-même
était sensible.

### 3.4 Vecteurs de conformance

`spec/cursor-vectors.json` contient la clé HMAC de test et trois triplets
(filtres, charge utile, token). Chaque implémentation doit avoir un test qui, pour
chaque vecteur :

1. recalcule l'empreinte à partir des filtres et retrouve `fingerprint` ;
2. produit exactement la chaîne `payload` ;
3. produit exactement la chaîne `token` ;
4. décode `token` et retrouve les valeurs de départ.

C'est ce test qui garantit qu'un curseur Go passe en Spring Boot et
réciproquement. En production, la clé vient de la configuration et n'est jamais
committée.

---

## 4. Les deux requêtes SQL, et pourquoi il en faut deux

Le piège documenté dans l'article : une requête unique qui gère la première page
et les suivantes via `($1::timestamptz IS NULL OR …)` s'effondre dès que le
driver bascule sur un plan générique. La borne cesse d'être une `Index Cond` et
devient un `Filter` — on a réinventé `OFFSET` avec la syntaxe du keyset.

Les deux implémentations émettent donc **deux requêtes distinctes**, et chacune a
un test d'intégration qui lit le plan d'exécution pour le prouver.

Première page, filtrée :

```sql
SELECT id, account_id, amount_cents, label, created_at
FROM transactions
WHERE account_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;
```

Pages suivantes, filtrées :

```sql
SELECT id, account_id, amount_cents, label, created_at
FROM transactions
WHERE account_id = $1
  AND (created_at, id) < ($2, $3)
ORDER BY created_at DESC, id DESC
LIMIT $4;
```

En tri ascendant, la borne devient `>` et le `ORDER BY` passe en `ASC` sur les
deux colonnes. Les colonnes vont **toujours dans le même sens** : un tri mixte
casserait la comparaison de n-uplets, et c'est précisément pourquoi `sort`
n'accepte que deux valeurs.

`LIMIT` reçoit toujours `limit + 1`. La ligne excédentaire ne sort jamais du
serveur : elle répond à `has_more` pour le prix d'une ligne, contre 101 ms pour
un `COUNT(*)`.
