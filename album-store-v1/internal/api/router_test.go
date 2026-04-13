package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	router := NewRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if got := rec.Body.String(); got != `{"status":"ok"}` {
		t.Fatalf("expected exact health body, got %q", got)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected JSON content type, got %q", got)
	}
}

func TestRouterBoundsMultipartMemory(t *testing.T) {
	router := NewRouter(Dependencies{})

	if router.MaxMultipartMemory != 8<<20 {
		t.Fatalf("expected max multipart memory %d, got %d", 8<<20, router.MaxMultipartMemory)
	}
}
