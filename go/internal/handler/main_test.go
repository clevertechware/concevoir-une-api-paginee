package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/config"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/logger"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// stubService records what the handler asked for and replays what it was told
// to return, so the tests below assert on parameter handling — the handler's
// only real job — rather than on pagination logic that belongs to the service.
type stubService struct {
	keysetPage domain.KeysetPage
	offsetPage domain.OffsetPage
	exportPage domain.ExportPage
	estimate   int64
	err        error

	gotQuery   domain.ListQuery
	gotLimit   int
	gotToken   string
	gotPage    int
	gotSize    int
	gotAccount int64
	gotAfterID int64
}

func (s *stubService) List(
	_ context.Context, q domain.ListQuery, limit int, token string,
) (domain.KeysetPage, error) {
	s.gotQuery, s.gotLimit, s.gotToken = q, limit, token
	return s.keysetPage, s.err
}

func (s *stubService) ListByOffset(
	_ context.Context, accountID int64, page, size int,
) (domain.OffsetPage, error) {
	s.gotAccount, s.gotPage, s.gotSize = accountID, page, size
	return s.offsetPage, s.err
}

func (s *stubService) Export(_ context.Context, afterID int64, limit int) (domain.ExportPage, error) {
	s.gotAfterID, s.gotLimit = afterID, limit
	return s.exportPage, s.err
}

func (s *stubService) CountEstimate(context.Context) (int64, error) {
	return s.estimate, s.err
}

// stubPinger answers the health check.
type stubPinger struct{ err error }

func (p stubPinger) Ping(context.Context) error { return p.err }

// newTestServer wires the real routing table on top of a stubbed service, so
// the tests exercise the URLs the contract publishes rather than a handler
// method called directly.
func newTestServer(service *stubService, db Pinger) *HTTPServer {
	log := logger.NewNoOpLogger()
	return NewHTTPServer(
		config.Server{Mode: gin.TestMode}, log, db, NewHTTPTransactionHandler(service, log),
	)
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
