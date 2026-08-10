package handler

import (
	"errors"
	"net/http"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
	"github.com/gin-gonic/gin"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/cursor"
)

// errorResponse is the common envelope of spec/contract.md §2.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// respondError maps an error to the status and code the contract prescribes.
//
// Anything unrecognised becomes a bare 500: the wrapped chain goes to the log,
// never to the response body, where it would leak table names and query
// fragments to the caller.
func respondError(c *gin.Context, log logger.Logger, err error) {
	ctx := c.Request.Context()
	status, code := statusFor(err)

	if status == http.StatusInternalServerError {
		log.ErrorContext(ctx, "request failed", "error", err)
		c.JSON(status, errorResponse{errorBody{Code: code, Message: "internal server error"}})
		return
	}

	log.WarnContext(ctx, "request rejected", "status", status, "code", code, "error", err)
	c.JSON(status, errorResponse{errorBody{Code: code, Message: err.Error()}})
}

func statusFor(err error) (status int, code string) {
	switch {
	// A well-formed request pointing at a position that no longer exists. 410,
	// not 400: that distinction is exactly what tells the client to restart its
	// walk rather than to fix its query.
	case errors.Is(err, cursor.ErrCursorExpired):
		return http.StatusGone, "cursor_expired"

	case errors.Is(err, cursor.ErrFilterMismatch):
		return http.StatusBadRequest, "cursor_filter_mismatch"

	case errors.Is(err, cursor.ErrInvalidCursor):
		return http.StatusBadRequest, "invalid_cursor"

	case errors.Is(err, domain.ErrInvalidLimit):
		return http.StatusBadRequest, "invalid_limit"

	case errors.Is(err, domain.ErrInvalidPage):
		return http.StatusBadRequest, "invalid_page"

	case errors.Is(err, domain.ErrInvalidSort):
		return http.StatusBadRequest, "invalid_sort"

	case errors.Is(err, domain.ErrInvalidAccountID):
		return http.StatusBadRequest, "invalid_account_id"

	case errors.Is(err, domain.ErrInvalidAfterID):
		return http.StatusBadRequest, "invalid_after_id"

	default:
		return http.StatusInternalServerError, "internal_error"
	}
}
