package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validYAML = `
server:
  host: ""
  port: 8080
  mode: debug
  shutdown_timeout: 10s
postgres:
  host: localhost
  port: 5432
  database: pagination
  user: postgres
  password: pagination
  sslmode: disable
  min_conns: 2
  max_conns: 10
logging:
  level: info
  format: text
cursor:
  key: dev-key
  ttl: 72h
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600))
	return dir
}

func TestLoad_ReadsTheCommittedFile(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeConfig(t, validYAML))

	require.NoError(t, err)
	assert.Equal(t, ":8080", cfg.Server.Addr())
	assert.Equal(t, 10*time.Second, cfg.Server.ShutdownTimeout)
	assert.Equal(t, "pagination", cfg.Postgres.Database)
	assert.Equal(t, "dev-key", cfg.Cursor.Key)
	assert.Equal(t, 72*time.Hour, cfg.Cursor.TTL)
}

// TestLoad_LetsTheEnvironmentWin is why the signing key can stay out of git:
// the committed value is a development default, and deployment overrides it.
//
// No t.Parallel() here: t.Setenv changes the environment of the whole test
// binary, which is exactly what every other Load test reads.
func TestLoad_LetsTheEnvironmentWin(t *testing.T) {
	t.Setenv("PAGINATION_CURSOR__KEY", "the-production-key")
	t.Setenv("PAGINATION_POSTGRES__PASSWORD", "s3cret")
	t.Setenv("PAGINATION_SERVER__PORT", "9090")

	cfg, err := Load(writeConfig(t, validYAML))

	require.NoError(t, err)
	assert.Equal(t, "the-production-key", cfg.Cursor.Key)
	assert.Equal(t, "s3cret", cfg.Postgres.Password)
	assert.Equal(t, 9090, cfg.Server.Port)
}

func TestLoad_RefusesToStartOnSettingsThatWouldFailLater(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "an empty cursor key would sign every token with nothing",
			content: strings.Replace(validYAML, "  key: dev-key", `  key: ""`, 1),
			wantErr: "cursor.key is required",
		},
		{
			name:    "a zero TTL would expire every cursor immediately",
			content: strings.Replace(validYAML, "  ttl: 72h", "  ttl: 0s", 1),
			wantErr: "cursor.ttl must be positive",
		},
		{
			name:    "a missing database host",
			content: strings.Replace(validYAML, "  host: localhost", `  host: ""`, 1),
			wantErr: "postgres.host is required",
		},
		{
			name:    "a pool whose maximum is under its minimum",
			content: strings.Replace(validYAML, "  max_conns: 10", "  max_conns: 1", 1),
			wantErr: "is lower than postgres.min_conns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Load(writeConfig(t, tt.content))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoad_FailsWhenTheFileIsMissing(t *testing.T) {
	t.Parallel()

	_, err := Load(t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), configFileName)
}

func TestDSN_BuildsTheConnectionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		postgres Postgres
		want     string
	}{
		{
			name:     "keeps an explicit sslmode",
			postgres: Postgres{Host: "db", Port: 5432, User: "u", Password: "p", Database: "d", SSLMode: "require"},
			want:     "host=db port=5432 user=u password=p dbname=d sslmode=require",
		},
		{
			name:     "defaults sslmode to disable",
			postgres: Postgres{Host: "db", Port: 5432, User: "u", Password: "p", Database: "d"},
			want:     "host=db port=5432 user=u password=p dbname=d sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.postgres.DSN())
		})
	}
}
