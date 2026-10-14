package web

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/radian-solusi/microservice-helpers/validate"
)

// captureLog redirects the stdlib logger to a buffer for the duration of fn
// and returns what was written. Every error-response path in this package
// must log through this logger — tests assert on its content.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)
	fn()
	return buf.String()
}

func TestErrorResponseLogsMethodPathStatusMessage(t *testing.T) {
	ctx, _ := newValidationCtx(t, "en")
	ctx.Request.Method = "POST"
	ctx.Request.URL.Path = "/create"
	out := captureLog(t, func() {
		ErrorResponse(ctx, BadRequest, "missing field x")
	})
	for _, want := range []string{"POST", "/create", "400", "missing field x"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q, got: %s", want, out)
		}
	}
}

func TestValidationErrorResponseLogsFieldDetails(t *testing.T) {
	ctx, _ := newValidationCtx(t, "en")
	ctx.Request.Method = "POST"
	ctx.Request.URL.Path = "/create"
	err := validate.NewFieldError("website", "url", errors.New("bad"))
	out := captureLog(t, func() {
		ValidationErrorResponse(ctx, err)
	})
	for _, want := range []string{"POST", "/create", "422", "website", "url"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q, got: %s", want, out)
		}
	}
}

func TestValidationErrorResponseLogsRawErrorWhenNotFieldLevel(t *testing.T) {
	// Non-validator errors fall back to "Invalid input data" in the response
	// (contract unchanged) but the raw cause must still be logged so it's
	// never silently swallowed.
	ctx, rec := newValidationCtx(t, "en")
	ctx.Request.Method = "POST"
	ctx.Request.URL.Path = "/create"
	cause := errors.New("json: cannot unmarshal string into Go struct field .foo of type int")
	out := captureLog(t, func() {
		ValidationErrorResponse(ctx, cause)
	})
	if !strings.Contains(out, "cannot unmarshal string") {
		t.Errorf("raw cause not logged: %s", out)
	}
	// response contract must stay the fallback message, not the raw error
	if strings.Contains(rec.Body.String(), "cannot unmarshal") {
		t.Error("raw error leaked into response body")
	}
}

func TestHandleErrorResponseLogsMappedCode(t *testing.T) {
	ctx, _ := newValidationCtx(t, "en")
	ctx.Request.Method = "DELETE"
	ctx.Request.URL.Path = "/pipeline/9"
	out := captureLog(t, func() {
		HandleErrorResponse(ctx, errors.New("not found in db"), func(error) int { return NotFound })
	})
	for _, want := range []string{"DELETE", "/pipeline/9", "404", "not found in db"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q, got: %s", want, out)
		}
	}
}

func TestSuccessResponseDoesNotLog(t *testing.T) {
	ctx, _ := newValidationCtx(t, "en")
	out := captureLog(t, func() {
		SendResponseData(ctx, Success, "ok", map[string]any{"id": 1})
	})
	if out != "" {
		t.Errorf("success response must not log, got: %s", out)
	}
}
