package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/config"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/handler"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/postgres"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/service"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet when configuration fails.
		logger.NewDefault().Error("server exited with an error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configDir := flag.String("config", ".", "directory containing application.yaml")
	flag.Parse()

	cfg, err := config.Load(*configDir)
	if err != nil {
		return err
	}

	log := logger.New(cfg.Logging)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.InfoContext(ctx, "connected to database", "database", cfg.Postgres.Database)

	repository := postgres.NewTransactionRepository(pool, log)
	transactions := service.NewTransactions(repository, cfg.Cursor, log)

	server := handler.NewHTTPServer(
		cfg.Server, log, repository, handler.NewHTTPTransactionHandler(transactions, log),
	)
	return server.Run(ctx)
}
