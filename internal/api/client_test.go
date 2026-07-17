package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientSendsBearerTokenAndParsesJSON(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/sites" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("User-Agent") != "nore-cli/1.2.3" {
			t.Errorf("user-agent = %q", request.Header.Get("User-Agent"))
		}
		_ = json.NewEncoder(response).Encode(map[string]any{"sites": []any{}})
	}))
	defer server.Close()
	client := Client{HTTPClient: server.Client(), Registry: server.URL, Version: "1.2.3"}
	var result map[string]any
	if err := client.Do(context.Background(), http.MethodGet, "/v1/cli/sites", "token", nil, &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["sites"]; !ok {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientPreservesAPIError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(response).Encode(map[string]any{
			"code":    "SCOPE_REQUIRED",
			"message": "CLI scope required: sites:read",
		})
	}))
	defer server.Close()
	client := Client{HTTPClient: server.Client(), Registry: server.URL}
	err := client.Do(context.Background(), http.MethodGet, "/v1/cli/sites", "token", nil, nil)
	var apiError *Error
	if !errors.As(err, &apiError) {
		t.Fatalf("Do() error = %v", err)
	}
	if apiError.Status != http.StatusForbidden || apiError.Code != "SCOPE_REQUIRED" {
		t.Fatalf("Do() error = %#v", apiError)
	}
}
