// Package handler exposes the application over HTTP using gin.
package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
	"github.com/gin-gonic/gin"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/config"
)

// Pinger reports whether a backing service is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HTTPServer owns the gin engine and the underlying http.Server.
type HTTPServer struct {
	server          *http.Server
	router          *gin.Engine
	logger          logger.Logger
	db              Pinger
	transactions    *HTTPTransactionHandler
	shutdownTimeout time.Duration
}

// NewHTTPServer builds the server and registers every route. Routes are wired
// here rather than in an exported method the caller has to remember to call.
func NewHTTPServer(cfg config.Server, log logger.Logger, db Pinger, transactions *HTTPTransactionHandler) *HTTPServer {
	gin.SetMode(ginMode(cfg.Mode))

	router := gin.New()
	router.Use(gin.Recovery(), requestLogger(log))

	s := &HTTPServer{
		router:          router,
		logger:          log,
		db:              db,
		transactions:    transactions,
		shutdownTimeout: cfg.ShutdownTimeout,
		server: &http.Server{
			Addr:              cfg.Addr(),
			Handler:           router,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
	s.setupRoutes()

	return s
}

// setupRoutes registers every route, with the claim of the article each one
// exists to demonstrate.
func (s *HTTPServer) setupRoutes() {
	s.router.GET("/healthz", s.health)

	v1Transactions := s.router.Group("/v1/transactions")
	{
		// ✅ The recommended list: opaque signed cursor, cost independent of depth.
		v1Transactions.GET("", s.transactions.list)

		// ❌ The counter-example: same data, same order, priced by depth. Here to
		// be measured against the one above, not to be reused.
		v1Transactions.GET("/offset", s.transactions.listByOffset)

		// ✅ The full walk, ordered on the immutable key: every row present when
		// the walk started is returned exactly once.
		v1Transactions.GET("/export", s.transactions.export)

		// The honest answer to "give me a total": an estimate, labelled as one.
		v1Transactions.GET("/count-estimate", s.transactions.countEstimate)
	}
}

// Run serves until ctx is canceled, then drains in-flight requests.
func (s *HTTPServer) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.logger.InfoContext(ctx, "HTTP server listening", "addr", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("shutting down HTTP server", "timeout", s.shutdownTimeout)

		// Deliberately detached from ctx: ctx is already cancelled, and the
		// point of this context is to give in-flight requests time to finish.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdownTimeout)
		defer cancel()

		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	}
}

// ServeHTTP exposes the router, so tests can drive the real routing table.
func (s *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// health reports whether the service can still reach its database.
func (s *HTTPServer) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		s.logger.ErrorContext(ctx, "health check failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// requestLogger emits one structured record per request through our logger,
// instead of gin's own line-oriented format.
func requestLogger(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		log.InfoContext(c.Request.Context(), "request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(start),
		)
	}
}

func ginMode(mode string) string {
	switch mode {
	case gin.ReleaseMode, gin.TestMode, gin.DebugMode:
		return mode
	default:
		return gin.ReleaseMode
	}
}
