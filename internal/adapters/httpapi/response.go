package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"ezbkTools/internal/app"
	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// Meta carries request-level metadata (e.g., request id).
type Meta struct {
	RequestID string `json:"request_id"`
}

// ErrorBody is a structured error returned to clients.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Envelope is a unified API response wrapper.
type Envelope struct {
	Data  any        `json:"data,omitempty"`
	Error *ErrorBody `json:"error,omitempty"`
	Meta  *Meta      `json:"meta,omitempty"`
}

var (
	errBadKeyType = errors.New("key_type має бути 'mcc' або 'bank_category'")
	errBadLimit   = errors.New("limit має бути додатним числом")
	errBadKind    = errors.New("kind має бути 'income', 'expense' або 'transfer'")
)

const (
	codeBadRequest      = "bad_request"
	codeValidationError = "validation_error"
	codeNotFound        = "not_found"
	codeConflict        = "conflict"
	codeInternalError   = "internal_error"
	codeUnauthorized    = "unauthorized"
	codeTooManyRequests = "too_many_requests"
)

func (s *Server) meta(c *gin.Context) *Meta {
	if gin.Mode() == gin.ReleaseMode {
		return nil
	}
	return &Meta{RequestID: requestid.Get(c)}
}

func (s *Server) respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Data: data, Meta: s.meta(c)})
}

func (s *Server) respondCreated(c *gin.Context, location string, data any) {
	if location != "" {
		c.Header("Location", location)
	}
	c.JSON(http.StatusCreated, Envelope{Data: data, Meta: s.meta(c)})
}

func (s *Server) respondNoContent(c *gin.Context) {
	// 204 must not include a body
	c.Status(http.StatusNoContent)
}

// classifyError maps an error to HTTP status and client-facing code in one place,
// so status and code can't drift apart.
func classifyError(err error) (int, string) {
	var (
		syntaxErr     *json.SyntaxError
		typeErr       *json.UnmarshalTypeError
		validationErr validator.ValidationErrors
	)

	switch {
	// TODO: доменні помилки з app/domain (errors.Is(err, app.ErrXxx)) → not_found / conflict / ...
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized, codeUnauthorized
	case errors.Is(err, ErrTooManyAttempts):
		return http.StatusTooManyRequests, codeTooManyRequests
	case errors.Is(err, errBadKeyType), errors.Is(err, errBadLimit), errors.Is(err, errBadKind),
		errors.Is(err, domain.ErrInvalidMCCRange),
		errors.Is(err, domain.ErrInvalidDirection),
		errors.Is(err, domain.ErrEmptyCategory),
		errors.Is(err, domain.ErrInvalidAccountMapping):
		return http.StatusBadRequest, codeValidationError
	case errors.Is(err, app.ErrRuleExists):
		return http.StatusConflict, codeConflict
	case errors.As(err, &validationErr):
		return http.StatusBadRequest, codeValidationError
	case errors.Is(err, io.EOF),
		errors.Is(err, io.ErrUnexpectedEOF),
		errors.As(err, &syntaxErr),
		errors.As(err, &typeErr):
		return http.StatusBadRequest, codeBadRequest
	default:
		return http.StatusInternalServerError, codeInternalError
	}
}

// errorMessage builds the client-facing message. Unmapped 5xx errors are hidden
// in release mode: they may carry SQL and other internals.
func errorMessage(err error, status int) string {
	if status >= http.StatusInternalServerError && gin.Mode() == gin.ReleaseMode {
		return "internal server error"
	}
	return err.Error()
}

// errorResponse sends a structured error envelope with an explicit HTTP status.
func (s *Server) errorResponse(c *gin.Context, status int, err error) {
	_, code := classifyError(err)

	// що сховали від клієнта, має лишитись у логах — інакше 500-ка стане невидимою
	if status >= http.StatusInternalServerError {
		logctx.From(c.Request.Context()).Error("request failed", "err", err)
	}

	c.JSON(status, Envelope{
		Error: &ErrorBody{Code: code, Message: errorMessage(err, status)},
		Meta:  s.meta(c),
	})
}

// respondError automatically maps the error to the correct HTTP status and sends the response.
func (s *Server) respondError(c *gin.Context, err error) {
	status, _ := classifyError(err)
	s.errorResponse(c, status, err)
}
