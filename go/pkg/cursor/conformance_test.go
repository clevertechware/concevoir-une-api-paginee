package cursor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// specVectors mirrors spec/cursor-vectors.json.
type specVectors struct {
	HMACKey string `json:"hmac_key_utf8"`
	Vectors []struct {
		Name    string `json:"name"`
		Filters struct {
			AccountID int64  `json:"account_id"`
			Status    string `json:"status"`
			Sort      string `json:"sort"`
		} `json:"filters"`
		Fingerprint string `json:"fingerprint"`
		Payload     string `json:"payload"`
		Token       string `json:"token"`
	} `json:"vectors"`
}

// TestConformance_MatchesTheSharedVectors is the test that makes the cursor a
// contract rather than an implementation detail.
//
// Every byte matters. If the payload gains a space, loses a decimal on the
// timestamp or reorders two keys, the signature changes and a token emitted by
// the Spring Boot implementation stops decoding here — silently, at the worst
// possible moment. So the assertion is on the exact strings, not on a
// round-trip: a round-trip only proves this implementation agrees with itself.
func TestConformance_MatchesTheSharedVectors(t *testing.T) {
	spec := loadVectors(t)
	key := []byte(spec.HMACKey)

	for _, vector := range spec.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			filters := Filters{
				AccountID: vector.Filters.AccountID,
				Status:    vector.Filters.Status,
				Sort:      vector.Filters.Sort,
			}
			assert.Equal(t, vector.Fingerprint, Fingerprint(filters), "filter fingerprint")

			decoded, err := DecodeAt(vector.Token, key, 0, time.Now())
			require.NoError(t, err, "the reference token must decode")

			assert.Equal(t, vector.Payload, decoded.canonicalPayload(), "canonical payload")
			assert.Equal(t, vector.Token, Encode(decoded, key), "signed token")

			assert.Equal(t, vector.Fingerprint, decoded.Fingerprint)
			assert.Equal(t, vector.Filters.Sort == "created_at:desc", decoded.Descending)
			assertPayloadRoundTrip(t, vector.Payload, decoded)
		})
	}
}

// TestConformance_TokenIsSignatureThenPayload pins the byte layout the other
// implementation relies on: 32 bytes of HMAC, then the payload, base64url
// without padding. The payload stays readable on purpose — it is signed, not
// encrypted.
func TestConformance_TokenIsSignatureThenPayload(t *testing.T) {
	spec := loadVectors(t)

	for _, vector := range spec.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			raw, err := base64.RawURLEncoding.DecodeString(vector.Token)
			require.NoError(t, err)

			require.Greater(t, len(raw), 32)
			assert.Equal(t, vector.Payload, string(raw[32:]))
		})
	}
}

// assertPayloadRoundTrip checks the decoded fields against the reference
// payload parsed independently, so a shared bug in canonicalPayload cannot make
// both sides of the comparison wrong in the same way.
func assertPayloadRoundTrip(t *testing.T, payload string, decoded Cursor) {
	t.Helper()

	var p wirePayload
	require.NoError(t, json.Unmarshal([]byte(payload), &p))

	assert.Equal(t, Version, p.Version)
	assert.Equal(t, p.ID, decoded.ID)
	assert.Equal(t, p.Descending, decoded.Descending)
	assert.Equal(t, p.Fingerprint, decoded.Fingerprint)
	assert.Equal(t, p.IssuedAt, decoded.IssuedAt.Unix())
	assert.Equal(t, p.CreatedAt, decoded.CreatedAt.UTC().Format(timeLayout))
}

func loadVectors(t *testing.T) specVectors {
	t.Helper()

	path := filepath.Join(repositoryRoot(t), "spec", "cursor-vectors.json")
	content, err := os.ReadFile(path)
	require.NoError(t, err, "the shared vectors live outside the Go module, at %s", path)

	var spec specVectors
	require.NoError(t, json.Unmarshal(content, &spec))
	require.NotEmpty(t, spec.Vectors)

	return spec
}

// repositoryRoot walks up to the Go module, then one level further: spec/ is
// shared with the Spring Boot implementation and belongs to neither.
func repositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err = os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Dir(dir)
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, fmt.Sprintf("no go.mod above %s", dir))
		dir = parent
	}
}
