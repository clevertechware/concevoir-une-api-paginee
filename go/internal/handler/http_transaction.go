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
	List(ctx context.Context, params domain.ListParams) (domain.KeysetPage, error)
	ListByOffset(ctx context.Context, params domain.OffsetParams) (domain.OffsetPage, error)
	Total(ctx context.Context, exact bool) (int64, error)
}

// HTTPTransactionHandler exposes the three listing endpoints of the contract.
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

type countEstimateResponse struct {
	Estimate int64 `json:"estimate"`
	// Exact is always false, and saying so is the point: a client that needs a
	// total gets an honest estimate rather than a number that cost 101 ms.
	Exact bool `json:"exact"`
}

// list serves GET /v1/transactions, the keyset walk.
func (h *HTTPTransactionHandler) list(c *gin.Context) {
	var request keysetListRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		respondError(c, h.logger, err)
		return
	}

	page, err := h.service.List(c.Request.Context(), request.params())
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
	var request offsetListRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		respondError(c, h.logger, err)
		return
	}

	page, err := h.service.ListByOffset(c.Request.Context(), request.params())
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	c.JSON(http.StatusOK, offsetListResponse{
		Data: transactionsResponse(page.Transactions),
		Page: offsetPageResponse{Page: page.Page, Size: page.Size, HasMore: page.HasMore},
	})
}

// total serves GET /v1/transactions/count-estimate.
func (h *HTTPTransactionHandler) total(c *gin.Context) {
	var request totalRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		respondError(c, h.logger, err)
		return
	}

	exact := request.Exact.value()
	estimate, err := h.service.Total(c.Request.Context(), exact)
	if err != nil {
		respondError(c, h.logger, err)
		return
	}

	c.JSON(http.StatusOK, countEstimateResponse{Estimate: estimate, Exact: exact})
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
