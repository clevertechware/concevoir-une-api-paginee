package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/config"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/handler/mocks"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// newTestServer wires the real routing table on top of a mocked service, so the
// tests exercise the URLs the contract publishes rather than a handler method
// called directly.
func newTestServer(service transactionService, db Pinger) *HTTPServer {
	log := logger.NewNoOpLogger()
	return NewHTTPServer(
		config.Server{Mode: gin.TestMode}, log, db, NewHTTPTransactionHandler(service, log),
	)
}

// idlePinger answers no call at all: every test but the health check leaves the
// database untouched, and an unexpected Ping would fail the test.
func idlePinger(t *testing.T) *mocks.Pinger {
	t.Helper()
	return mocks.NewPinger(t)
}

func get(t *testing.T, server *HTTPServer, target string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func decodeBody[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()

	var body T
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}
