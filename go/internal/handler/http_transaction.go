package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
	"github.com/gin-gonic/gin"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
)

type transactionService interface {
	List(ctx context.Context, q domain.ListQuery, limit int, token string) (domain.KeysetPage, error)
	ListByOffset(ctx context.Context, accountID int64, page, size int) (domain.OffsetPage, error)
	Export(ctx context.Context, afterID int64, limit int) (domain.ExportPage, error)
	CountEstimate(ctx context.Context) (int64, error)
}

// HTTPTransactionHandler exposes the four listing endpoints of the contract.
type HTTPTransactionHandler struct {
	service transactionService
	logger  logger.Logger
}

// NewHTTPTransactionHandler creates the transaction handler.
func NewHTTPTransactionHandler(service transactionService, log logger.Logger) *HTTPTransactionHandler {
	return &HTTPTransactionHandler{service: service, logger: log}
}

// transactionResponse is one row on the wire.
//
// ID is a string. A bigint goes past JavaScript's Number.MAX_SAFE_INTEGER, and
// a client reading it back as a number would silently lose the low bits — the
// kind of bug that only shows up once the table is big enough to matter.
type transactionResponse struct {
	ID          string `json:"id"`
	AccountID   int64  `json:"account_id"`
	AmountCents int64  `json:"amount_cents"`
	Label       string `json:"label"`
	CreatedAt   string `json:"created_at"`
}

type keysetPageResponse struct {
	// Next is null at the end of the walk, and it is the only authority on that.
	Next    *string `json:"next"`
	HasMore bool    `json:"has_more"`
}

type keysetListResponse struct {
	Data []transactionResponse `json:"data"`
	Page keysetPageResponse    `json:"page"`
}

type offsetPageResponse struct {
	Page    int  `json:"page"`
	Size    int  `json:"size"`
	HasMore bool `json:"has_more"`
}

type offsetListResponse struct {
	Data []transactionResponse `json:"data"`
	Page offsetPageResponse    `json:"page"`
}

type exportPageResponse struct {
	NextAfterID *string `json:"next_after_id"`
	HasMore     bool    `json:"has_more"`
}

type exportListResponse struct {
	Data []transactionResponse `json:"data"`
	Page exportPageResponse    `json:"page"`
}

type countEstimateResponse struct {
	Estimate int64 `json:"estimate"`
	// Exact is always false, and saying so is the point: a client that needs a
	// total gets an honest estimate rather than a number that cost 101 ms.
	Exact bool `json:"exact"`
}

// list serves GET /v1/transactions, the keyset walk.
func (h *HTTPTransactionHandler) list(c *gin.Context) {
	accountID, err := accountIDParam(c)
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	sort, err := domain.ParseSort(c.Query("sort"))
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	limit, err := limitParam(c, "limit", domain.DefaultLimit, domain.MaxLimit)
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	query := domain.ListQuery{AccountID: accountID, Status: c.Query("status"), Sort: sort}

	page, err := h.service.List(c.Request.Context(), query, limit, c.Query("cursor"))
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	c.JSON(http.StatusOK, keysetListResponse{
		Data: transactionsResponse(page.Transactions),
		Page: keysetPageResponse{Next: nullable(page.Next), HasMore: page.HasMore},
	})
}

// listByOffset serves GET /v1/transactions/offset, the counter-example.
func (h *HTTPTransactionHandler) listByOffset(c *gin.Context) {
	accountID, err := accountIDParam(c)
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	pageNumber, err := pageParam(c)
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	size, err := limitParam(c, "size", domain.DefaultLimit, domain.MaxLimit)
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	page, err := h.service.ListByOffset(c.Request.Context(), accountID, pageNumber, size)
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	c.JSON(http.StatusOK, offsetListResponse{
		Data: transactionsResponse(page.Transactions),
		Page: offsetPageResponse{Page: page.Page, Size: page.Size, HasMore: page.HasMore},
	})
}

// export serves GET /v1/transactions/export, the full walk on the immutable key.
func (h *HTTPTransactionHandler) export(c *gin.Context) {
	afterID, err := int64Param(c, "after_id", domain.ErrInvalidAfterID)
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	limit, err := limitParam(c, "limit", domain.DefaultExportLimit, domain.MaxExportLimit)
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	page, err := h.service.Export(c.Request.Context(), afterID, limit)
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	var next *string
	if page.NextAfterID > 0 {
		next = nullable(strconv.FormatInt(page.NextAfterID, 10))
	}

	c.JSON(http.StatusOK, exportListResponse{
		Data: transactionsResponse(page.Transactions),
		Page: exportPageResponse{NextAfterID: next, HasMore: page.HasMore},
	})
}

// countEstimate serves GET /v1/transactions/count-estimate.
func (h *HTTPTransactionHandler) countEstimate(c *gin.Context) {
	estimate, err := h.service.CountEstimate(c.Request.Context())
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	c.JSON(http.StatusOK, countEstimateResponse{Estimate: estimate, Exact: false})
}

func transactionsResponse(transactions []domain.Transaction) []transactionResponse {
	out := make([]transactionResponse, 0, len(transactions))
	for _, t := range transactions {
		out = append(out, transactionResponse{
			ID:          strconv.FormatInt(t.ID, 10),
			AccountID:   t.AccountID,
			AmountCents: t.AmountCents,
			Label:       t.Label,
			CreatedAt:   t.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return out
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// limitParam reads a page size and caps it. Over the maximum is not an error:
// the contract caps, it does not reject.
func limitParam(c *gin.Context, name string, fallback, maxAllowed int) (int, error) {
	raw := c.Query(name)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, domain.ErrInvalidLimit
	}

	return domain.CapLimit(value, fallback, maxAllowed), nil
}

func pageParam(c *gin.Context) (int, error) {
	raw := c.Query("page")
	if raw == "" {
		return 1, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, domain.ErrInvalidPage
	}
	return value, nil
}

func accountIDParam(c *gin.Context) (int64, error) {
	return int64Param(c, "account_id", domain.ErrInvalidAccountID)
}

// int64Param reads a positive bigint query parameter. Absent means 0, which is
// what both the filter fingerprint and the export walk use for "no value".
func int64Param(c *gin.Context, name string, invalid error) (int64, error) {
	raw := c.Query(name)
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, invalid
	}
	return value, nil
}
