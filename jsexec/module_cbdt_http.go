package jsexec

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// CbdtHttpShim is the JS shim that esbuild injects for "cbdt:http".
const CbdtHttpShim = `export default __cbdt_http;`

// defaultHTTPTimeout is the default timeout for HTTP requests if none is specified.
const defaultHTTPTimeout = 5 * time.Second

// SetupCbdtHttp registers the cbdt:http module object on the runtime.
func SetupCbdtHttp(vm *Runtime) {
	httpObj := vm.NewObject()
	httpObj.Set("get", httpMethod(vm, http.MethodGet))
	httpObj.Set("post", httpMethod(vm, http.MethodPost))
	httpObj.Set("put", httpMethod(vm, http.MethodPut))
	httpObj.Set("delete", httpMethod(vm, http.MethodDelete))
	vm.Set("__cbdt_http", httpObj)
}

// httpMethod returns a Goja function that performs an HTTP request with the given method.
func httpMethod(vm *Runtime, method string) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		url := call.Argument(0).String()
		if url == "" {
			panic(vm.NewGoError(fmt.Errorf("http.%s requires a URL argument", strings.ToLower(method))))
		}

		var body string
		headers := make(map[string]string)
		timeout := defaultHTTPTimeout

		// Parse optional options object.
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			optsObj := call.Argument(1).ToObject(vm.Runtime)

			if v := optsObj.Get("headers"); v != nil && !goja.IsUndefined(v) {
				headersObj := v.ToObject(vm.Runtime)
				for _, key := range headersObj.Keys() {
					headers[key] = headersObj.Get(key).String()
				}
			}

			if v := optsObj.Get("body"); v != nil && !goja.IsUndefined(v) {
				body = v.String()
			}

			if v := optsObj.Get("timeout"); v != nil && !goja.IsUndefined(v) {
				timeoutMs := v.ToInteger()
				timeout = time.Duration(timeoutMs) * time.Millisecond
			}
		}

		// Build the HTTP request.
		var bodyReader io.Reader
		if body != "" {
			bodyReader = strings.NewReader(body)
		}

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("failed to create %s request: %w", method, err)))
		}

		for key, val := range headers {
			req.Header.Set(key, val)
		}

		// Execute with timeout.
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			// Check if the error is a timeout.
			if isTimeoutError(err) {
				panic(ThrowCodedError(vm.Runtime, "timeout",
					fmt.Sprintf("%s %s timed out after %s", method, url, timeout)))
			}
			panic(vm.NewGoError(fmt.Errorf("http %s %s failed: %w", method, url, err)))
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("failed to read response body: %w", err)))
		}

		// Build response headers map.
		respHeaders := vm.NewObject()
		for key, vals := range resp.Header {
			if len(vals) > 0 {
				respHeaders.Set(key, vals[0])
			}
		}

		// Build the response object.
		result := vm.NewObject()
		result.Set("status", resp.StatusCode)
		result.Set("statusText", resp.Status)
		result.Set("headers", respHeaders)
		result.Set("body", string(respBody))

		return result
	}
}

// isTimeoutError checks if an error is a timeout.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	type timeoutErr interface {
		Timeout() bool
	}
	if te, ok := err.(timeoutErr); ok && te.Timeout() {
		return true
	}
	return strings.Contains(err.Error(), "context deadline exceeded")
}
