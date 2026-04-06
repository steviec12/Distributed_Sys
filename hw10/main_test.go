package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type recordResponse struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version uint64 `json:"version"`
}

func resetState() {
	storeMu.Lock()
	defer storeMu.Unlock()

	store = make(map[string]Record)
	version = 0
}

func TestSetThenGet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetState()

	router := setupRouter()

	setReq := httptest.NewRequest(http.MethodPost, "/set", bytes.NewBufferString(`{"key":"a","value":"hello"}`))
	setReq.Header.Set("Content-Type", "application/json")
	setResp := httptest.NewRecorder()
	router.ServeHTTP(setResp, setReq)

	if setResp.Code != http.StatusCreated {
		t.Fatalf("expected status %d from /set, got %d", http.StatusCreated, setResp.Code)
	}

	var setBody recordResponse
	if err := json.Unmarshal(setResp.Body.Bytes(), &setBody); err != nil {
		t.Fatalf("failed to decode /set response: %v", err)
	}

	if setBody.Key != "a" {
		t.Fatalf("expected key %q from /set, got %q", "a", setBody.Key)
	}

	if setBody.Value != "hello" {
		t.Fatalf("expected value %q from /set, got %q", "hello", setBody.Value)
	}

	if setBody.Version != 1 {
		t.Fatalf("expected version %d from /set, got %d", 1, setBody.Version)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/get?key=a", nil)
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)

	if getResp.Code != http.StatusOK {
		t.Fatalf("expected status %d from /get, got %d", http.StatusOK, getResp.Code)
	}

	var getBody recordResponse
	if err := json.Unmarshal(getResp.Body.Bytes(), &getBody); err != nil {
		t.Fatalf("failed to decode /get response: %v", err)
	}

	if getBody.Key != "a" {
		t.Fatalf("expected key %q from /get, got %q", "a", getBody.Key)
	}

	if getBody.Value != "hello" {
		t.Fatalf("expected value %q from /get, got %q", "hello", getBody.Value)
	}

	if getBody.Version != 1 {
		t.Fatalf("expected version %d from /get, got %d", 1, getBody.Version)
	}
}
