// Package config loads the application configuration from application.yaml,
// then lets environment variables override it so that the cursor signing key
// never has to live in a committed file.
package config

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	configFileName = "application.yaml"

	// envPrefix scopes the environment variables we consider. Nested keys use a
	// double underscore: PAGINATION_CURSOR__KEY overrides cursor.key.
	envPrefix = "PAGINATION_"
	envNested = "__"
)

// Server holds the HTTP server settings.
type Server struct {
	Host string `koanf:"host"`
	Port int    `koanf:"port"`
	// Mode is the gin mode: debug, release, or test.
	Mode string `koanf:"mode"`
	// ShutdownTimeout bounds how long in-flight requests get to finish.
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`
}

// Addr returns the listen address for the HTTP server.
func (s Server) Addr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

// Postgres holds the database connection settings.
//
// There is no migration setting: the schema is shared infrastructure applied by
// the container from sql/01-schema.sql, and this application only reads.
type Postgres struct {
	Host     string `koanf:"host"`
	Port     int    `koanf:"port"`
	Database string `koanf:"database"`
	User     string `koanf:"user"`
	Password string `koanf:"password"`
	SSLMode  string `koanf:"sslmode"`
	MinConns int32  `koanf:"min_conns"`
	MaxConns int32  `koanf:"max_conns"`
}

// DSN returns the keyword/value connection string consumed by pgxpool.
func (p Postgres) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.User, p.Password, p.Database, p.sslMode(),
	)
}

func (p Postgres) sslMode() string {
	if p.SSLMode == "" {
		return "disable"
	}
	return p.SSLMode
}

// Cursor holds what it takes to sign and to expire a pagination cursor.
type Cursor struct {
	// Key signs the payload. Rotating it invalidates every token in flight,
	// which is the intended blast radius: clients restart their walk.
	Key string `koanf:"key"`
	// TTL is how long a position stays resumable. Past it the answer is 410,
	// never 400: the request was well formed, the position is simply gone.
	TTL time.Duration `koanf:"ttl"`
}

// Application is the root configuration.
type Application struct {
	Server   Server               `koanf:"server"`
	Postgres Postgres             `koanf:"postgres"`
	Logging  logger.LoggingConfig `koanf:"logging"`
	Cursor   Cursor               `koanf:"cursor"`
}

// Load reads application.yaml from dir, then applies PAGINATION_-prefixed
// environment variables on top so deployment always wins over the committed file.
func Load(dir string) (*Application, error) {
	k := koanf.New(".")

	if err := k.Load(file.Provider(filepath.Join(dir, configFileName)), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("loading %s: %w", configFileName, err)
	}

	envProvider := env.Provider(".", env.Opt{
		Prefix: envPrefix,
		TransformFunc: func(key, value string) (string, any) {
			key = strings.ToLower(strings.TrimPrefix(key, envPrefix))
			return strings.ReplaceAll(key, envNested, "."), value
		},
	})
	if err := k.Load(envProvider, nil); err != nil {
		return nil, fmt.Errorf("loading environment variables: %w", err)
	}

	var app Application
	if err := k.UnmarshalWithConf("", &app, koanf.UnmarshalConf{Tag: "koanf"}); err != nil {
		return nil, fmt.Errorf("unmarshalling configuration: %w", err)
	}

	if err := app.validate(); err != nil {
		return nil, err
	}

	return &app, nil
}

// validate rejects the settings that would otherwise fail much later, with a
// far less obvious error.
func (a Application) validate() error {
	switch {
	case a.Postgres.Host == "":
		return fmt.Errorf("postgres.host is required")
	case a.Postgres.Database == "":
		return fmt.Errorf("postgres.database is required")
	case a.Postgres.MaxConns < a.Postgres.MinConns:
		return fmt.Errorf("postgres.max_conns (%d) is lower than postgres.min_conns (%d)",
			a.Postgres.MaxConns, a.Postgres.MinConns)
	case a.Server.Port <= 0:
		return fmt.Errorf("server.port must be positive, got %d", a.Server.Port)
	case a.Cursor.Key == "":
		// An unsigned cursor is a forgeable position, so refuse to start rather
		// than sign every token with an empty key.
		return fmt.Errorf("cursor.key is required")
	case a.Cursor.TTL <= 0:
		return fmt.Errorf("cursor.ttl must be positive, got %s", a.Cursor.TTL)
	}
	return nil
}
