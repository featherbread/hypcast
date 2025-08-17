// Package rpc implements a barebones RPC framework based on HTTP and JSON.
//
// Clients invoke RPC methods by making an HTTP POST request to a well known
// path, and may provide parameters via a single JSON-encoded value in the
// request body. RPC responses include an appropriate HTTP status code, and may
// include a response body containing a single JSON-encoded value.
//
// POST is the only allowed HTTP method for RPC requests, both with and without
// parameters. Requests with parameters must include a Content-Type header with
// the value "application/json". Specific RPC requests may limit the size of
// allowed request bodies to conserve server resources.
//
// This framework is incomplete for Internet-facing production use.
// For example, RPC handlers should not be exposed to web browsers without
// stronger cross-site request forgery enforcement.
package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
)

// Handle wraps an RPC handler into an [http.Handler] that follows the RPC
// framework conventions noted in the package documentation.
//
// When the client provides a JSON parameters value in the request body,
// the RPC framework decodes it following standard [json.Unmarshal] rules.
// It buffers the request body in memory before decoding, which may not be
// memory-efficent for some use cases. [WithLimitedBodyBuffer] may wrap one or
// more RPC handlers to limit the sizes of allowed request bodies.
//
// When the RPC handler returns a Go error as the response body, the framework
// encodes it as a JSON object with an "Error" key containing the error
// message. Otherwise, when the body is non-nil, the framework encodes it to
// JSON following standard [json.Marshal] rules.
func Handle[T any](h func(r *http.Request, params T) (code int, body any)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		blocked := respondIfBadMethod(w, r)
		if blocked {
			return
		}

		var rbody bytes.Buffer
		switch b := r.Body.(type) {
		case *bufferedBody:
			rbody = b.Buffer
		default:
			_, err := rbody.ReadFrom(r.Body)
			if err != nil {
				respondError(w, errReadingBody)
				return
			}
		}

		var params T
		if rbody.Len() > 0 {
			if r.Header.Get("Content-Type") != "application/json" {
				respondError(w, errInvalidBodyType)
				return
			}
			err := json.Unmarshal(rbody.Bytes(), &params)
			if err != nil {
				respondError(w, errInvalidBody)
				return
			}
		}

		code, body := h(r, params)
		respond(w, code, body)
	})
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

// bufferedBody acts as a sentinel for [Handle] that a request body is already
// buffered, and implements [io.ReadCloser] so [http.Request] can carry it.
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
