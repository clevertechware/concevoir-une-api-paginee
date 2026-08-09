// Package testutil starts the PostgreSQL container the integration tests run
// against. None of what those tests assert — an Index Cond rather than a
// Filter, a block count that does not grow with depth, a walk that does not
// drift under concurrent inserts — can be faked with a mock: they are claims
// about what the database actually does.
package testutil

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/config"
)

const (
	image    = "postgres:18-alpine"
	database = "pagination_test"
	username = "postgres"
	password = "postgres"
)

// Postgres is a running container with the shared schema applied.
type Postgres struct {
	Config config.Postgres
	Pool   *pgxpool.Pool
}

// shared is the single container for the test binary. Go runs each package as
// its own process, so one per package is also one per process.
var shared *Postgres

// RunWithPostgres starts the container, runs the tests, then tears it down.
// Call it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testutil.RunWithPostgres(m)) }
//
// In short mode it starts nothing, so `go test -short` needs no Docker.
func RunWithPostgres(m *testing.M) int {
	// testing.Short() is only meaningful once the flags are parsed, and m.Run()
	// has not done that yet.
	flag.Parse()
	if testing.Short() {
		return m.Run()
	}

	ctx := context.Background()

	pg, terminate, err := start(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting postgres container: %v\n", err)
		return 1
	}
	shared = pg

	code := m.Run()

	if err = terminate(); err != nil {
		fmt.Fprintf(os.Stderr, "terminating postgres container: %v\n", err)
	}
	return code
}

// Shared returns the container started by RunWithPostgres, skipping the test in short mode.
func Shared(t *testing.T) *Postgres {
	t.Helper()

	if testing.Short() {
		t.Skip("integration test: needs Docker, skipped in short mode")
	}
	if shared == nil {
		t.Fatal("no container: TestMain must call testutil.RunWithPostgres")
	}
	return shared
}

func start(ctx context.Context) (*Postgres, func() error, error) {
	// The schema is shared infrastructure: the application never applies it, and
	// neither does a migration tool. The tests read the very same file the
	// compose entrypoint uses, so a schema change cannot pass the suite while
	// breaking the running stack.
	schema, err := readSharedSchema()
	if err != nil {
		return nil, nil, err
	}

	container, err := tcpostgres.Run(ctx, image,
		tcpostgres.WithDatabase(database),
		tcpostgres.WithUsername(username),
		tcpostgres.WithPassword(password),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, nil, err
	}

	terminate := func() error {
		return testcontainers.TerminateContainer(container)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, nil, err
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, nil, err
	}

	cfg := config.Postgres{
		Host:     host,
		Port:     int(port.Num()),
		Database: database,
		User:     username,
		Password: password,
		SSLMode:  "disable",
		MinConns: 2,
		MaxConns: 10,
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, nil, err
	}
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConns = cfg.MaxConns

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		return nil, nil, err
	}
	if _, err = pool.Exec(ctx, schema); err != nil {
		return nil, nil, fmt.Errorf("applying %s: %w", schemaFile, err)
	}

	return &Postgres{Config: cfg, Pool: pool}, func() error {
		pool.Close()
		return terminate()
	}, nil
}

const schemaFile = "sql/01-schema.sql"

func readSharedSchema() (string, error) {
	root, err := RepositoryRoot()
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(filepath.Join(root, schemaFile))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", schemaFile, err)
	}
	return string(content), nil
}

// RepositoryRoot returns the directory holding sql/ and spec/, one level above
// the Go module.
func RepositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err = os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Dir(dir), nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("module root not found above %s", dir)
		}
		dir = parent
	}
}
