package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/radian-solusi/microservice-helpers/validate"
)

func newValidationCtx(t *testing.T, lang string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("GET", "/", nil)
	SetLanguageCtx(ctx, lang)
	return ctx, rec
}

func TestValidationErrorResponseShape(t *testing.T) {
	ctx, rec := newValidationCtx(t, "id")
	err := validate.NewFieldError("website", "url", errors.New("bad"))
	ValidationErrorResponse(ctx, err)
	if rec.Code != ValidationError {
		t.Fatalf("code %d", rec.Code)
	}
	if !ctx.IsAborted() {
		t.Fatal("expected abort")
	}
	var body ResponseDefault
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status {
		t.Fatal("status must be false")
	}
	data, _ := body.Data.(map[string]any)
	if data["website"] != "website bukan URL yang valid" {
		t.Errorf("got %+v", body.Data)
	}
	if body.Message != "website bukan URL yang valid" {
		t.Errorf("summary got %q", body.Message)
	}
}

func TestValidationErrorResponseEnglish(t *testing.T) {
	ctx, rec := newValidationCtx(t, "en")
	ValidationErrorResponse(ctx, validate.NewFieldError("website", "url", errors.New("bad")))
	var body ResponseDefault
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	data, _ := body.Data.(map[string]any)
	if data["website"] != "website is not a valid URL" {
		t.Errorf("got %+v", body.Data)
	}
}

func TestValidationErrorResponseUnknownErrorNoRawDetails(t *testing.T) {
	ctx, rec := newValidationCtx(t, "en")
	ValidationErrorResponse(ctx, errors.New("db connection refused at 10.0.0.1"))
	if rec.Code != ValidationError {
		t.Fatalf("code %d", rec.Code)
	}
	body := rec.Body.String()
	if containsAny(body, "db connection refused", "10.0.0.1") {
		t.Errorf("raw detail leaked: %s", body)
	}
	var parsed ResponseDefault
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	if parsed.Message != "Invalid input data" {
		t.Errorf("got message %q", parsed.Message)
	}
}

func TestValidationErrorResponseNilError(t *testing.T) {
	ctx, rec := newValidationCtx(t, "en")
	ValidationErrorResponse(ctx, nil)
	if rec.Code != ValidationError {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestIsMaxBytesErrorDetectsOverflow(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", strings.NewReader("0123456789"))
	req.Body = http.MaxBytesReader(rec, io.NopCloser(req.Body), 5)
	_, err := io.ReadAll(req.Body)
	if !IsMaxBytesError(err) {
		t.Fatalf("expected MaxBytesError, got %v (%T)", err, err)
	}
}

func TestIsMaxBytesErrorFalseForOtherErrors(t *testing.T) {
	if IsMaxBytesError(errors.New("boom")) {
		t.Fatal("expected false for unrelated error")
	}
}

func TestPayloadTooLargeResponseNamesLimit(t *testing.T) {
	ctx, rec := newValidationCtx(t, "en")
	PayloadTooLargeResponse(ctx, 50*1024*1024)
	if rec.Code != PayloadTooLarge {
		t.Fatalf("code %d", rec.Code)
	}
	if !ctx.IsAborted() {
		t.Fatal("expected abort")
	}
	var body ResponseDefault
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status {
		t.Fatal("status must be false")
	}
	if !containsAny(body.Message, "50") {
		t.Errorf("message must mention the limit, got %q", body.Message)
	}
}

func TestPayloadTooLargeResponseIndonesian(t *testing.T) {
	ctx, rec := newValidationCtx(t, "id")
	PayloadTooLargeResponse(ctx, 5*1024*1024)
	var body ResponseDefault
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Message != "Ukuran body permintaan melebihi batas 5MB" {
		t.Errorf("got %q", body.Message)
	}
}

func TestValidationErrorResponseWithRefRoutesMaxBytesErrorTo413(t *testing.T) {
	ctx, rec := newValidationCtx(t, "en")
	rec2 := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", strings.NewReader("0123456789"))
	req.Body = http.MaxBytesReader(rec2, io.NopCloser(req.Body), 5)
	_, readErr := io.ReadAll(req.Body)
	ValidationErrorResponseWithRef(ctx, readErr, nil)
	if rec.Code != PayloadTooLarge {
		t.Fatalf("code %d, want 413", rec.Code)
	}
	var body ResponseDefault
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !containsAny(body.Message, "MB") {
		t.Errorf("expected limit message, got %q", body.Message)
	}
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if n != "" && len(haystack) >= len(n) {
			for i := 0; i+len(n) <= len(haystack); i++ {
				if haystack[i:i+len(n)] == n {
					return true
				}
			}
		}
	}
	return false
}
