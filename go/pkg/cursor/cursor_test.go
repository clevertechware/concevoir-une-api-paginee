package cursor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testKey   = []byte("concevoir-une-api-paginee-test-key")
	testEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

func testCursor() Cursor {
	return Cursor{
		CreatedAt:   time.Date(2021, 5, 3, 19, 32, 40, 0, time.UTC),
		ID:          9999998,
		Descending:  true,
		Fingerprint: Fingerprint(Filters{Sort: "created_at:desc"}),
		IssuedAt:    testEpoch,
	}
}

func TestEncodeDecode_PreservesThePosition(t *testing.T) {
	original := testCursor()

	decoded, err := DecodeAt(Encode(original, testKey), testKey, time.Hour, testEpoch)

	require.NoError(t, err)
	assert.True(t, original.CreatedAt.Equal(decoded.CreatedAt))
	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.Descending, decoded.Descending)
	assert.Equal(t, original.Fingerprint, decoded.Fingerprint)
	assert.True(t, original.IssuedAt.Equal(decoded.IssuedAt))
}

// TestDecode_RejectsAnythingButAnUntouchedToken covers the claim that a cursor
// cannot be forged. Every case answers the same ErrInvalidCursor: telling them
// apart would tell an attacker which check to work on next.
func TestDecode_RejectsAnythingButAnUntouchedToken(t *testing.T) {
	valid := Encode(testCursor(), testKey)

	tests := []struct {
		name    string
		token   string
		key     []byte
		wantErr error
	}{
		{
			name:    "accepts the untouched token",
			token:   valid,
			key:     testKey,
			wantErr: nil,
		},
		{
			name:    "rejects a token with one byte changed in the payload",
			token:   flipLastByte(t, valid),
			key:     testKey,
			wantErr: ErrInvalidCursor,
		},
		{
			name:    "rejects a token signed with another key",
			token:   Encode(testCursor(), []byte("another-key")),
			key:     testKey,
			wantErr: ErrInvalidCursor,
		},
		{
			name:    "rejects text that is not base64url",
			token:   "not a cursor!",
			key:     testKey,
			wantErr: ErrInvalidCursor,
		},
		{
			name:    "rejects a token too short to hold a signature",
			token:   base64.RawURLEncoding.EncodeToString([]byte("short")),
			key:     testKey,
			wantErr: ErrInvalidCursor,
		},
		{
			name:    "rejects a signature with an empty payload behind it",
			token:   base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
			key:     testKey,
			wantErr: ErrInvalidCursor,
		},
		{
			name:    "rejects a correctly signed payload of an unknown version",
			token:   signRaw(`{"v":2,"c":"2021-05-03T19:32:40.000000Z","i":1,"d":true,"f":"x","t":1767225600}`),
			key:     testKey,
			wantErr: ErrInvalidCursor,
		},
		{
			name:    "rejects a correctly signed payload that is not JSON",
			token:   signRaw(`not json`),
			key:     testKey,
			wantErr: ErrInvalidCursor,
		},
		{
			name:    "rejects a timestamp that is not the canonical format",
			token:   signRaw(`{"v":1,"c":"2021-05-03T19:32:40Z","i":1,"d":true,"f":"x","t":1767225600}`),
			key:     testKey,
			wantErr: ErrInvalidCursor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeAt(tt.token, tt.key, time.Hour, testEpoch)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestDecode_ExpiresOnTTL is why the contract answers 410 rather than 400: the
// token is perfectly valid, it just points at a position that is too old to
// resume from.
func TestDecode_ExpiresOnTTL(t *testing.T) {
	token := Encode(testCursor(), testKey)

	tests := []struct {
		name    string
		ttl     time.Duration
		now     time.Time
		wantErr error
	}{
		{
			name: "accepts a cursor inside its TTL",
			ttl:  72 * time.Hour,
			now:  testEpoch.Add(71 * time.Hour),
		},
		{
			name: "accepts a cursor exactly at its TTL",
			ttl:  72 * time.Hour,
			now:  testEpoch.Add(72 * time.Hour),
		},
		{
			name:    "rejects a cursor past its TTL",
			ttl:     72 * time.Hour,
			now:     testEpoch.Add(72*time.Hour + time.Second),
			wantErr: ErrCursorExpired,
		},
		{
			name: "skips the check when no TTL is given",
			ttl:  0,
			now:  testEpoch.Add(10 * 365 * 24 * time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeAt(token, testKey, tt.ttl, tt.now)
			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestFingerprint_DistinguishesEveryFilter is what makes a cursor replayed on
// another query detectable rather than silently answered.
func TestFingerprint_DistinguishesEveryFilter(t *testing.T) {
	base := Filters{AccountID: 42, Status: "SETTLED", Sort: "created_at:desc"}

	tests := []struct {
		name  string
		other Filters
	}{
		{name: "another account", other: Filters{AccountID: 43, Status: "SETTLED", Sort: "created_at:desc"}},
		{name: "no account at all", other: Filters{Status: "SETTLED", Sort: "created_at:desc"}},
		{name: "another status", other: Filters{AccountID: 42, Status: "PENDING", Sort: "created_at:desc"}},
		{name: "the other sort direction", other: Filters{AccountID: 42, Status: "SETTLED", Sort: "created_at:asc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEqual(t, Fingerprint(base), Fingerprint(tt.other))
		})
	}

	assert.Equal(t, Fingerprint(base), Fingerprint(base), "the same filters always give the same fingerprint")
}

// TestCanonicalPayload_WritesTheContractedShape guards the three properties
// json.Marshal would silently break: the key order, the absence of whitespace,
// and six decimals on the timestamp whatever its actual precision.
func TestCanonicalPayload_WritesTheContractedShape(t *testing.T) {
	tests := []struct {
		name   string
		cursor Cursor
		want   string
	}{
		{
			name:   "keeps the contracted key order and no whitespace",
			cursor: testCursor(),
			want: `{"v":1,"c":"2021-05-03T19:32:40.000000Z","i":9999998,"d":true,` +
				`"f":"jlkDlANVKaQ","t":1767225600}`,
		},
		{
			name: "keeps six decimals when the timestamp has none",
			cursor: Cursor{
				CreatedAt:   time.Date(2015, 1, 1, 0, 0, 20, 0, time.UTC),
				ID:          1,
				Fingerprint: "x",
				IssuedAt:    testEpoch,
			},
			want: `{"v":1,"c":"2015-01-01T00:00:20.000000Z","i":1,"d":false,"f":"x","t":1767225600}`,
		},
		{
			name: "keeps six decimals when the timestamp has microseconds",
			cursor: Cursor{
				CreatedAt:   time.Date(2015, 1, 1, 0, 0, 20, 123456000, time.UTC),
				ID:          1,
				Fingerprint: "x",
				IssuedAt:    testEpoch,
			},
			want: `{"v":1,"c":"2015-01-01T00:00:20.123456Z","i":1,"d":false,"f":"x","t":1767225600}`,
		},
		{
			name: "converts a non-UTC timestamp before formatting it",
			cursor: Cursor{
				CreatedAt:   time.Date(2015, 1, 1, 2, 0, 20, 0, time.FixedZone("CET", 2*3600)),
				ID:          1,
				Fingerprint: "x",
				IssuedAt:    testEpoch,
			},
			want: `{"v":1,"c":"2015-01-01T00:00:20.000000Z","i":1,"d":false,"f":"x","t":1767225600}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cursor.canonicalPayload())
		})
	}
}

func TestWriteJSONString_EscapesWhatJSONRequires(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "leaves a base64url fingerprint alone", input: "jlkDlANVKaQ", want: `"jlkDlANVKaQ"`},
		{name: "escapes a double quote", input: `a"b`, want: `"a\"b"`},
		{name: "escapes a backslash", input: `a\b`, want: `"a\\b"`},
		{name: "escapes a newline", input: "a\nb", want: `"a\nb"`},
		{
			name:  "escapes a control character as a unicode escape",
			input: "a\x01b",
			want:  fmt.Sprintf(`"a%s0001b"`, `\u`),
		},
		{name: "leaves non-ASCII as is", input: "café", want: `"café"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			writeJSONString(&b, tt.input)
			assert.Equal(t, tt.want, b.String())
		})
	}
}

func flipLastByte(t *testing.T, token string) string {
	t.Helper()

	raw, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)

	raw[len(raw)-1] ^= 0x01
	return base64.RawURLEncoding.EncodeToString(raw)
}

// signRaw signs an arbitrary payload with the test key, so the decoder's
// post-signature checks can be exercised on payloads Encode would never produce.
func signRaw(payload string) string {
	mac := hmac.New(sha256.New, testKey)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(append(mac.Sum(nil), payload...))
}
