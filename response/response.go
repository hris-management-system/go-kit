package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	apperror "github.com/hris-management-system/go-kit/errors"
)

// Err writes the appropriate HTTP error response.
// If err is an *apperror.AppError the status and message come from the error code.
// Any other error falls back to 500.
func Err(c *gin.Context, err error) {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		d := apperror.Detail(ae.Code)
		c.JSON(ae.HttpStatus(), Problem{
			Title:    http.StatusText(ae.HttpStatus()),
			Status:   ae.HttpStatus(),
			Detail:   d.Message,
			Code:     http.StatusText(ae.HttpStatus()),
			Instance: c.Request.URL.Path,
		})
		return
	}
	Internal(c, err.Error())
}

// Success: just return the data, no envelope.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, data)
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Paginated success — when you need it
type Page[T any] struct {
	Data     []T   `json:"data"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func Paginated[T any](c *gin.Context, items []T, total int64, page, size int) {
	c.JSON(http.StatusOK, Page[T]{
		Data: items, Total: total, Page: page, PageSize: size,
	})
}

// Error — RFC 7807 Problem Details
type Problem struct {
	Type     string       `json:"type,omitempty"`
	Title    string       `json:"title,omitempty"`
	Status   int          `json:"status,omitempty"`
	Detail   string       `json:"detail,omitempty"`
	Instance string       `json:"instance,omitempty"`
	Code     string       `json:"code,omitempty"`   // app-specific error code
	Errors   []FieldError `json:"errors,omitempty"` // for validation errors
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func Error(c *gin.Context, status int, code, title, detail string) {
	c.JSON(status, Problem{
		Title:    title,
		Status:   status,
		Detail:   detail,
		Code:     code,
		Instance: c.Request.URL.Path,
	})
}

// Convenience helpers
func BadRequest(c *gin.Context, code, detail string) {
	Error(c, http.StatusBadRequest, code, "Bad Request", detail)
}

func NotFound(c *gin.Context, code, detail string) {
	Error(c, http.StatusNotFound, code, "Not Found", detail)
}

func Unauthorized(c *gin.Context, code, detail string) {
	Error(c, http.StatusUnauthorized, code, "Unauthorized", detail)
}

func Forbidden(c *gin.Context, code, detail string) {
	Error(c, http.StatusForbidden, code, "Forbidden", detail)
}

func Internal(c *gin.Context, detail string) {
	Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal Server Error", detail)
}

// Validation error helper
func ValidationFailed(c *gin.Context, errs []FieldError) {
	c.JSON(http.StatusUnprocessableEntity, Problem{
		Title:    "Validation Failed",
		Status:   http.StatusUnprocessableEntity,
		Code:     "VALIDATION_FAILED",
		Errors:   errs,
		Instance: c.Request.URL.Path,
	})
}
