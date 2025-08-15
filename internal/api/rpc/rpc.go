// Package rpc implements a barebones RPC framework based on HTTP and JSON.
//
// Clients invoke RPC methods by making an HTTP POST request to a well known
// path, and may provide parameters via a single JSON-encoded value in the
// request body. RPC responses include an appropriate HTTP status code, and may
// include a response body containing a single JSON-encoded value.
//
// No HTTP method other than POST is accepted for RPC requests, even those that
// do not require parameters. The maximum size of RPC request bodies may be
// limited to conserve server resources. Requests with parameters must include a
// Content-Type header with the value "application/json".
//
// This framework is not considered acceptable for Internet-facing production
// use. For example, the Content-Type enforcement described above is the only
// mitigation against cross-site request forgery attacks.
// (TODO: Consider adopting http.CrossOriginProtection from Go 1.25.)
package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// DefaultMaxRequestBodySize is the default MaxRequestBodySize set by [NewHandler].
const DefaultMaxRequestBodySize int64 = 1024

// HandlerFunc is a type for functions that handle RPC calls initiated by HTTP
// clients, accepting parameters decoded from JSON and returning an HTTP status
// code and optional JSON-encodable result body.
//
// When the client provides a JSON parameters value in the request body, the RPC
// framework decodes it using standard json.Unmarshal rules. When the body
// returned by the handler is a Go error, the framework encodes it as a JSON
// object with an "Error" key containing the stringified error message.
// Otherwise, when the body is non-nil, the framework encodes it to JSON
// following standard json.Marshal rules.
type HandlerFunc[T any] func(r *http.Request, params T) (code int, body any)

type Handler[T any] struct {
	// Handle serves RPC requests.
	Handle HandlerFunc[T]

	// MaxRequestBodySize is the maximum size of the HTTP body in an RPC request.
	// Requests whose body exceeds this size will fail without being handled.
	MaxRequestBodySize int64
}

// NewHandler wraps an RPC handler function with default settings.
func NewHandler[T any](handle HandlerFunc[T]) Handler[T] {
	return Handler[T]{
		Handle:             handle,
		MaxRequestBodySize: DefaultMaxRequestBodySize,
	}
}

// ServeHTTP implements http.Handler for an RPC handler function.
func (h Handler[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Add("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var code int
	var body any
	if params, err := readRPCParams[T](r, h.MaxRequestBodySize); err == nil {
		code, body = h.Handle(r, params)
	} else {
		code, body = errorHTTPCode(err), err
	}

	if berr, ok := body.(error); ok {
		body = struct{ Error string }{berr.Error()}
	}

	if body == nil {
		w.WriteHeader(code)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(body)
}

type httpError struct {
	HTTPCode int
	Message  string
}

func (h httpError) Error() string { return h.Message }

var (
	errReadingBody     = httpError{http.StatusInternalServerError, "unable to read RPC body"}
	errBodyTooLarge    = httpError{http.StatusRequestEntityTooLarge, "RPC body exceeded maximum size"}
	errInvalidBodyType = httpError{http.StatusUnsupportedMediaType, "must have Content-Type: application/json"}
	errInvalidBody     = httpError{http.StatusBadRequest, "unable to decode RPC body"}
)

func readRPCParams[T any](r *http.Request, sizeLimit int64) (T, error) {
	var body bytes.Buffer
	n, err := body.ReadFrom(io.LimitReader(r.Body, sizeLimit+1))
	switch {
	case err != nil:
		return *new(T), errReadingBody
	case n == 0:
		return *new(T), nil
	case n > sizeLimit:
		return *new(T), errBodyTooLarge
	}

	if r.Header.Get("Content-Type") != "application/json" {
		return *new(T), errInvalidBodyType
	}

	var params T
	if err := json.Unmarshal(body.Bytes(), &params); err != nil {
		return *new(T), errInvalidBody
	}
	return params, err
}

func errorHTTPCode(err error) int {
	var herr httpError
	if errors.As(err, &herr) {
		return herr.HTTPCode
	}
	return http.StatusInternalServerError
}
