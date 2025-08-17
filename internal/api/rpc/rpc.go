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
	"net/http"
)

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
}

// NewHandler wraps an RPC handler function with default settings.
func NewHandler[T any](handle HandlerFunc[T]) Handler[T] {
	return Handler[T]{Handle: handle}
}

// ServeHTTP implements http.Handler for an RPC handler function.
func (h Handler[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	blocked := respondIfBadMethod(w, r)
	if blocked {
		return
	}

	var rbody bytes.Buffer
	var err error
	switch b := r.Body.(type) {
	case *bufferedBody:
		rbody = b.Buffer
	default:
		_, err = rbody.ReadFrom(r.Body)
		if err != nil {
			err = errReadingBody
		}
	}

	var params T
	if rbody.Len() > 0 {
		if r.Header.Get("Content-Type") != "application/json" {
			err = errInvalidBodyType
		} else {
			err = json.Unmarshal(rbody.Bytes(), &params)
			if err != nil {
				err = errInvalidBody
			}
		}
	}
	if err != nil {
		respondError(w, err)
		return
	}

	code, body := h.Handle(r, params)
	respond(w, code, body)
}

// WithLimitedBodyBuffer limits the size of request bodies passed to the
// wrapped [http.Handler], rejecting large requests with an HTTP 413 response
// and JSON error body following the conventions of the RPC framework.
// It does this by buffering the request body in memory up to the limit,
// which may not be memory-efficient for some use cases.
//
// WithLimitedBodyBuffer is designed for use with RPC framework handlers,
// and may impose additional requirements (e.g. allowed HTTP methods)
// as noted in the package documentation.
func WithLimitedBodyBuffer(limit int64, handle http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		blocked := respondIfBadMethod(w, r)
		if blocked {
			return
		}

		var rbody bytes.Buffer
		_, err := rbody.ReadFrom(http.MaxBytesReader(w, r.Body, limit))
		r.Body.Close()
		if err != nil {
			switch err.(type) {
			case *http.MaxBytesError:
				respondError(w, errBodyTooLarge)
			default:
				respondError(w, errReadingBody)
			}
			return
		}

		r.Body = &bufferedBody{rbody}
		handle.ServeHTTP(w, r)
	})
}

type bufferedBody struct{ bytes.Buffer }

func (bufferedBody) Close() error { return nil }

func respondIfBadMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		w.Header().Add("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return true
	}
	return false
}

func respond(w http.ResponseWriter, code int, body any) {
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

func respondError(w http.ResponseWriter, err error) {
	respond(w, errorHTTPCode(err), err)
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

func errorHTTPCode(err error) int {
	var herr httpError
	if errors.As(err, &herr) {
		return herr.HTTPCode
	}
	return http.StatusInternalServerError
}
