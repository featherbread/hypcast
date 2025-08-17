package rpc_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/featherbread/hypcast/internal/api/rpc"
)

func Example() {
	setEnabled := func(enabled bool) { /* do something useful */ }

	mux := http.NewServeMux()
	mux.Handle("/rpc/setstatus",
		rpc.Handle(func(_ *http.Request, params struct{ Enabled *bool }) (code int, body any) {
			if params.Enabled == nil {
				return http.StatusBadRequest, errors.New(`missing "Enabled" parameter`)
			}
			setEnabled(*params.Enabled)
			return http.StatusNoContent, nil
		}))

	csrf := http.NewCrossOriginProtection()
	handler := csrf.Handler(
		rpc.WithLimitedBodyBuffer(1024,
			mux))

	req := httptest.NewRequest(
		http.MethodPost, "/rpc/setstatus", strings.NewReader(`{"Enabled": true}`))

	req.Header.Add("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	fmt.Println(resp.Result().StatusCode)
	// Output: 204
}

func TestRPC(t *testing.T) {
	const rpcTestBodySizeLimit = 32
	handler := func(_ *http.Request, _ struct{}) (code int, body any) {
		return http.StatusNoContent, nil
	}

	jsonHeaders := http.Header{"Content-Type": {"application/json"}}

	testCases := []struct {
		Description string
		Method      string
		Body        string
		Headers     http.Header
		WantCode    int
		WantHeaders http.Header
	}{
		{
			Description: "empty body",
			WantCode:    http.StatusNoContent,
		},
		{
			Description: "body with maximum length",
			Body:        `{"Message":"123456789012345678"}`,
			Headers:     jsonHeaders,
			WantCode:    http.StatusNoContent,
		},
		{
			Description: "body too long by 1 character",
			Body:        `{"Message":"1234567890123456789"}`,
			Headers:     jsonHeaders,
			WantCode:    http.StatusRequestEntityTooLarge,
			WantHeaders: jsonHeaders,
		},
		{
			Description: "missing Content-Type header",
			Body:        `{"Valid":false}`,
			WantCode:    http.StatusUnsupportedMediaType,
			WantHeaders: jsonHeaders,
		},
		{
			Description: "invalid JSON body",
			Body:        `{{{]]]`,
			Headers:     jsonHeaders,
			WantCode:    http.StatusBadRequest,
			WantHeaders: jsonHeaders,
		},
		{
			Description: "invalid HTTP method",
			Method:      http.MethodGet,
			WantCode:    http.StatusMethodNotAllowed,
			WantHeaders: http.Header{"Allow": {http.MethodPost}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			method := tc.Method
			if method == "" {
				method = http.MethodPost
			}

			req := httptest.NewRequest(method, "/", strings.NewReader(tc.Body))
			req.Header = tc.Headers

			// TODO: Separate tests for RPC handler wrapping and body size limits.
			rh := rpc.WithLimitedBodyBuffer(rpcTestBodySizeLimit, rpc.Handle(handler))

			resp := httptest.NewRecorder()
			rh.ServeHTTP(resp, req)

			if resp.Result().StatusCode != tc.WantCode {
				t.Errorf("wrong status: got %d, want %d", resp.Result().StatusCode, tc.WantCode)
			}

			diff := cmp.Diff(tc.WantHeaders, resp.Result().Header, cmpopts.EquateEmpty())
			if diff != "" {
				t.Errorf("wrong headers (-want +got)\n%s", diff)
			}

			var body strings.Builder
			io.Copy(&body, resp.Result().Body) // Intentionally best-effort.
			if body.Len() > 0 {
				t.Logf("response body: %s", body.String())
			}
		})
	}
}
