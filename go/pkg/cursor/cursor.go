// Package cursor implements the opaque pagination cursor described in
// spec/contract.md.
//
// It is public on purpose. The cursor is a contract boundary, not an
// implementation detail: a token emitted here is consumed unchanged by the
// Spring Boot implementation of the same demo, and the other way round.
// Everything that decides whether that holds — the canonical payload, the
// filter fingerprint, the signature — lives in this file.
package cursor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Version is the payload format version. A token carrying any other value is
// rejected, which is what lets the format change without breaking the tokens
// already in circulation.
const Version = 1

// timeLayout is RFC 3339 UTC with exactly six decimals, the precision of a
// PostgreSQL timestamptz. Fixed width on both sides: Go's RFC3339Nano would
// drop trailing zeros and no longer match what Java writes.
const timeLayout = "2006-01-02T15:04:05.000000Z"

// fingerprintBytes is how much of the SHA-256 digest ends up in the token.
// Eight bytes is not a security boundary (the HMAC is). It only has to make an
// accidental collision between two filter sets implausible.
const fingerprintBytes = 8

var (
	// ErrInvalidCursor covers every unreadable token: bad base64, truncated
	// payload, failed signature, unknown version. They collapse into one error
	// on purpose, so the response never tells an attacker which check failed.
	ErrInvalidCursor = errors.New("invalid cursor")

	// ErrCursorExpired means the token was well-formed but issued too long ago.
	// The caller answers 410, not 400: the request was fine, the position is gone.
	ErrCursorExpired = errors.New("cursor expired")

	// ErrFilterMismatch means the token was issued for another set of filters.
	ErrFilterMismatch = errors.New("cursor filters do not match the request")
)

// Filters are the request attributes a cursor is bound to. AccountID is 0 when
// no tenant filter is applied, and Sort holds the normalised sort value.
type Filters struct {
	AccountID int64
	Status    string
	Sort      string
}

// Fingerprint returns the filter checksum carried in the token's "f" field.
//
//	base64url_nopad( SHA256("<account_id>|<status>|<sort>")[0:8] )
func Fingerprint(f Filters) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s", f.AccountID, f.Status, f.Sort)))
	return base64.RawURLEncoding.EncodeToString(sum[:fingerprintBytes])
}

// Cursor is a decoded position in a keyset walk.
type Cursor struct {
	// CreatedAt and ID are the sort key of the last row handed to the client.
	CreatedAt time.Time
	ID        int64
	// Descending records the walk direction, so a token cannot be replayed backwards.
	Descending bool
	// Fingerprint binds the token to the filters that produced it.
	Fingerprint string
	// IssuedAt is what the expiry check compares against.
	IssuedAt time.Time
}

// Encode signs the canonical payload and returns the token:
//
//	base64url_nopad( HMAC_SHA256(key, payload) || payload )
//
// It cannot fail: the payload is written here rather than marshalled, so there
// is no serialiser to disagree with.
func Encode(c Cursor, key []byte) string {
	payload := []byte(c.canonicalPayload())

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)

	return base64.RawURLEncoding.EncodeToString(append(mac.Sum(nil), payload...))
}

// Decode verifies a token against the wall clock. A cursor older than ttl
// yields ErrCursorExpired; everything else yields ErrInvalidCursor.
func Decode(token string, key []byte, ttl time.Duration) (Cursor, error) {
	return DecodeAt(token, key, ttl, time.Now())
}

// DecodeAt is Decode with the clock supplied by the caller, so that expiry can
// be tested without waiting three days. A ttl of zero or less skips the expiry
// check entirely, which is what the conformance test needs to read vectors
// whose issue date is fixed for good.
func DecodeAt(token string, key []byte, ttl time.Duration, now time.Time) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) <= sha256.Size {
		return Cursor{}, ErrInvalidCursor
	}

	signature, payload := raw[:sha256.Size], raw[sha256.Size:]

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	// Constant time: a byte-by-byte comparison leaks how much of a forged
	// signature was right, which is enough to reconstruct the rest.
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return Cursor{}, ErrInvalidCursor
	}

	var p wirePayload
	if err = json.Unmarshal(payload, &p); err != nil || p.Version != Version {
		return Cursor{}, ErrInvalidCursor
	}

	createdAt, err := time.Parse(timeLayout, p.CreatedAt)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}

	issuedAt := time.Unix(p.IssuedAt, 0).UTC()
	if ttl > 0 && now.Sub(issuedAt) > ttl {
		return Cursor{}, ErrCursorExpired
	}

	return Cursor{
		CreatedAt:   createdAt,
		ID:          p.ID,
		Descending:  p.Descending,
		Fingerprint: p.Fingerprint,
		IssuedAt:    issuedAt,
	}, nil
}

// wirePayload mirrors the canonical object for reading only. Key order is
// irrelevant here, which is exactly why it cannot be reused for writing.
type wirePayload struct {
	Version     int    `json:"v"`
	CreatedAt   string `json:"c"`
	ID          int64  `json:"i"`
	Descending  bool   `json:"d"`
	Fingerprint string `json:"f"`
	IssuedAt    int64  `json:"t"`
}

// canonicalPayload writes the payload byte for byte:
//
//	{"v":1,"c":"<created_at>","i":<id>,"d":<bool>,"f":"<fingerprint>","t":<issued_at>}
//
// json.Marshal is not an option. It would order the keys after the struct
// fields rather than after the contract, and it would format the timestamp with
// RFC3339Nano, which drops trailing zeros. Either one changes the bytes, and
// changing the bytes changes the signature.
func (c Cursor) canonicalPayload() string {
	var b strings.Builder

	b.WriteString(`{"v":`)
	b.WriteString(strconv.Itoa(Version))
	b.WriteString(`,"c":`)
	writeJSONString(&b, c.CreatedAt.UTC().Format(timeLayout))
	b.WriteString(`,"i":`)
	b.WriteString(strconv.FormatInt(c.ID, 10))
	b.WriteString(`,"d":`)
	b.WriteString(strconv.FormatBool(c.Descending))
	b.WriteString(`,"f":`)
	writeJSONString(&b, c.Fingerprint)
	b.WriteString(`,"t":`)
	b.WriteString(strconv.FormatInt(c.IssuedAt.UTC().Unix(), 10))
	b.WriteString(`}`)

	return b.String()
}

// writeJSONString appends s as a JSON string literal.
//
// Spelled out rather than delegated to encoding/json, whose HTML escaping turns
// the angle brackets and the ampersand into unicode escapes where Jackson
// leaves them alone. None of them can appear in the fields we serialise, but
// the payload is a cross-language contract and such a divergence would only
// ever surface as a signature mismatch, far from its cause.
func writeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
