package command

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/norehq/cli/internal/config"
	"github.com/norehq/cli/internal/store"
)

func TestSiteCommandsRequireSelectionInJSONMode(t *testing.T) {
	configureTestEnvironment(t, config.DefaultRegistry)
	for _, args := range [][]string{
		{"site", "get", "--json"},
		{"post", "list", "--json"},
		{"post", "get", "--post", "post-id", "--json"},
		{"release", "create", "--json"},
		{"release", "list", "--json"},
		{"release", "get", "--release", "release-id", "--json"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if exitCode := Execute(args, &stdout, &stderr); exitCode == 0 {
			t.Fatalf("Execute(%q) = %d, stdout = %s", args, exitCode, stdout.String())
		}
		var result struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(stderr.Bytes(), &result); err != nil {
			t.Fatalf("Execute(%q) stderr is not JSON: %v", args, err)
		}
		if result.Error.Code != "SITE_SELECTION_REQUIRED" {
			t.Fatalf("Execute(%q) error = %#v", args, result.Error)
		}
		if !strings.Contains(result.Error.Message, "nore site list --json") {
			t.Fatalf("Execute(%q) message = %q", args, result.Error.Message)
		}
	}
}

func TestResolveSiteUsesImplicitSiteListAndSelector(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != "/v1/cli/sites" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(response).Encode(map[string]any{
			"sites": []map[string]any{
				{
					"id":         "site-docs",
					"ident":      "docs",
					"previewUrl": "https://docs.example.com",
					"status":     "READY",
				},
				{"id": "site-blog", "ident": "blog", "status": "READY"},
			},
		})
	}))
	defer server.Close()
	configureTestEnvironment(t, server.URL)
	t.Setenv("NORE_TOKEN", "person-token")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	selectorCalls := 0
	application := &app{
		configStore:     config.DefaultStore(),
		credentialStore: store.DefaultStore(),
		noColor:         true,
		stderr:          &stderr,
		stdout:          &stdout,
		siteSelector: func(_ context.Context, sites []siteSummary) (string, error) {
			selectorCalls++
			if len(sites) != 2 {
				t.Fatalf("sites = %#v", sites)
			}
			if label := siteLabel(sites[0]); label != "docs · READY · https://docs.example.com" {
				t.Fatalf("siteLabel() = %q", label)
			}
			return sites[1].ID, nil
		},
	}
	site, err := application.resolveSite(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if site != "site-blog" || selectorCalls != 1 || requests.Load() != 1 {
		t.Fatalf("site = %q, selector calls = %d, requests = %d", site, selectorCalls, requests.Load())
	}
	if !strings.Contains(stderr.String(), "No --site was provided. Select a site to continue.") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "nore site list --json") {
		t.Fatalf("stderr contains agent hint: %s", stderr.String())
	}
}

func TestResolveSiteSkipsImplicitListForExplicitSite(t *testing.T) {
	var stderr bytes.Buffer
	application := &app{stderr: &stderr}
	site, err := application.resolveSite(context.Background(), "  docs  ", false)
	if err != nil {
		t.Fatal(err)
	}
	if site != "docs" || stderr.Len() != 0 {
		t.Fatalf("site = %q, stderr = %q", site, stderr.String())
	}
}

func TestResolveSiteDoesNotPromptInMachineModes(t *testing.T) {
	for _, test := range []struct {
		name           string
		json           bool
		nonInteractive bool
		selector       siteSelector
		wantAgentHint  bool
	}{
		{name: "json", json: true, selector: unexpectedSiteSelector(t), wantAgentHint: true},
		{
			name:           "non-interactive",
			nonInteractive: true,
			selector:       unexpectedSiteSelector(t),
			wantAgentHint:  true,
		},
		{name: "non-tty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			application := &app{
				json:         test.json,
				stderr:       &stderr,
				siteSelector: test.selector,
			}
			_, err := application.resolveSite(context.Background(), "", test.nonInteractive)
			view, _ := errorView(err)
			if view.Code != "SITE_SELECTION_REQUIRED" {
				t.Fatalf("error = %#v", view)
			}
			hasAgentHint := strings.Contains(view.Message, "nore site list --json")
			if hasAgentHint != test.wantAgentHint {
				t.Fatalf("message = %q, want agent hint = %t", view.Message, test.wantAgentHint)
			}
		})
	}
}

func TestResolveSiteRejectsEmptyImplicitSiteList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(response).Encode(map[string]any{"sites": []any{}})
	}))
	defer server.Close()
	configureTestEnvironment(t, server.URL)
	t.Setenv("NORE_TOKEN", "person-token")
	var stderr bytes.Buffer
	application := &app{
		configStore:     config.DefaultStore(),
		credentialStore: store.DefaultStore(),
		noColor:         true,
		stderr:          &stderr,
		stdout:          &bytes.Buffer{},
		siteSelector:    unexpectedSiteSelector(t),
	}
	_, err := application.resolveSite(context.Background(), "", false)
	view, _ := errorView(err)
	if view.Code != "NO_SITES_AVAILABLE" {
		t.Fatalf("error = %#v", view)
	}
}

func TestResolveSiteDoesNotPrintSelectionHintBeforeAuthentication(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		response.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	configureTestEnvironment(t, server.URL)
	var stderr bytes.Buffer
	application := &app{
		configStore:     config.DefaultStore(),
		credentialStore: store.DefaultStore(),
		noColor:         true,
		stderr:          &stderr,
		stdout:          &bytes.Buffer{},
		siteSelector:    unexpectedSiteSelector(t),
	}
	_, err := application.resolveSite(context.Background(), "", false)
	view, _ := errorView(err)
	if view.Code != "NOT_LOGGED_IN" {
		t.Fatalf("error = %#v", view)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func TestExplicitSiteSkipsImplicitSiteList(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/v1/cli/sites/docs/posts" {
			t.Errorf("path = %q", request.URL.Path)
		}
		_ = json.NewEncoder(response).Encode(map[string]any{
			"page":       1,
			"posts":      []any{},
			"total":      0,
			"totalPages": 0,
		})
	}))
	defer server.Close()
	configureTestEnvironment(t, server.URL)
	t.Setenv("NORE_TOKEN", "person-token")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute(
		[]string{"post", "list", "--site", "docs", "--json"},
		&stdout,
		&stderr,
	); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func unexpectedSiteSelector(t *testing.T) siteSelector {
	t.Helper()
	return func(context.Context, []siteSummary) (string, error) {
		t.Fatal("site selector was called")
		return "", nil
	}
}
