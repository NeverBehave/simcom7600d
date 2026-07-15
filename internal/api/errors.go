package api

import (
	"net/http"
)

// apiError is the wire envelope returned by every error response.
//
// The JSON shape is {"error":{"code","message","details"}}. The struct uses
// an inner anonymous-struct field so that's exactly what gets marshalled.
// Status is exposed via GetStatus() to satisfy huma.StatusError.
type apiError struct {
	status  int
	Wrapped struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details,omitempty"`
	} `json:"error"`
}

func newAPIError(status int, code, message string, details any) *apiError {
	e := &apiError{status: status}
	e.Wrapped.Code = code
	e.Wrapped.Message = message
	e.Wrapped.Details = details
	return e
}

// Error implements error.
func (e *apiError) Error() string { return e.Wrapped.Message }

// GetStatus implements huma.StatusError.
func (e *apiError) GetStatus() int { return e.status }

// codeForStatus maps a numeric status to the symbolic code used in the envelope.
// Mirrors the table from the spec doc §9.6.
func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusTooManyRequests:
		return "too_many_requests"
	case http.StatusBadGateway:
		return "modem_error"
	case http.StatusServiceUnavailable:
		return "modem_unavailable"
	case http.StatusGatewayTimeout:
		return "modem_timeout"
	}
	if status >= 500 {
		return "internal"
	}
	return "error"
}
