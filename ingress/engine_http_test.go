package ingress_test

import (
	ingress "aaa/ingress"
	"aaa/ingress/builder"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPEngineLookupBackendSingleHostFilters(t *testing.T) {
	root := t.TempDir()
	store, err := ingress.NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	exact := newBackend(t, "backend-exact")
	prefix := newBackend(t, "backend-prefix")
	regex := newBackend(t, "backend-regex")
	query := newBackend(t, "backend-query")
	header := newBackend(t, "backend-header")

	err = store.WriteHTTP("example.com", builder.NewHTTPRoute().
		WithName("example.com").
		AddPolicy(builder.NewHTTPPolicy().WithBackend(exact.URL).WithExactPath("/exact")).
		AddPolicy(builder.NewHTTPPolicy().WithBackend(prefix.URL).WithPrefixPath("/api")).
		AddPolicy(builder.NewHTTPPolicy().WithBackend(regex.URL).WithRegexPath("^/items/[0-9]+$")).
		AddPolicy(builder.NewHTTPPolicy().WithBackend(query.URL).WithQuery("env", "prod")).
		AddPolicy(builder.NewHTTPPolicy().WithBackend(header.URL).WithHeader("x-region", "us")))
	if err != nil {
		t.Fatalf("WriteHTTP() error = %v", err)
	}

	engine := ingress.NewHTTPEngine(ingress.HTTPEngineOptions{Root: root})

	testCases := []struct {
		name    string
		target  string
		host    string
		headers map[string]string
		want    string
	}{
		{name: "exact path", target: "http://example.com/exact", host: "example.com", want: "backend-exact"},
		{name: "prefix path", target: "http://example.com/api/v1/users", host: "example.com", want: "backend-prefix"},
		{name: "regex path", target: "http://example.com/items/42", host: "example.com", want: "backend-regex"},
		{name: "query", target: "http://example.com/search?env=prod", host: "example.com", want: "backend-query"},
		{name: "header", target: "http://example.com/headers", host: "example.com", headers: map[string]string{"x-region": "us"}, want: "backend-header"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.target, nil)
			req.Host = tc.host
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			rec := httptest.NewRecorder()
			engine.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("ServeHTTP() status = %d, want %d", rec.Code, http.StatusOK)
			}
			if body := rec.Body.String(); body != tc.want {
				t.Fatalf("ServeHTTP() body = %q, want %q", body, tc.want)
			}
		})
	}
}

func TestHTTPEngineLookupBackendWildcardAndSingleHostPriority(t *testing.T) {
	root := t.TempDir()
	store, err := ingress.NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	wildcard := newBackend(t, "wildcard-backend")
	single := newBackend(t, "single-backend")

	err = store.WriteHTTP("*.example.com", builder.NewHTTPRoute().
		WithName("*.example.com").
		AddPolicy(builder.NewHTTPPolicy().WithBackend(wildcard.URL).WithPrefixPath("/")))
	if err != nil {
		t.Fatalf("WriteHTTP(wildcard) error = %v", err)
	}

	err = store.WriteHTTP("api.example.com", builder.NewHTTPRoute().
		WithName("api.example.com").
		AddPolicy(builder.NewHTTPPolicy().WithBackend(single.URL).WithPrefixPath("/")))
	if err != nil {
		t.Fatalf("WriteHTTP(single) error = %v", err)
	}

	engine := ingress.NewHTTPEngine(ingress.HTTPEngineOptions{Root: root})

	reqSingle := httptest.NewRequest("GET", "http://api.example.com/", nil)
	reqSingle.Host = "api.example.com"
	recSingle := httptest.NewRecorder()
	engine.Handler().ServeHTTP(recSingle, reqSingle)
	if recSingle.Code != http.StatusOK {
		t.Fatalf("ServeHTTP(single) status = %d, want %d", recSingle.Code, http.StatusOK)
	}
	if body := recSingle.Body.String(); body != "single-backend" {
		t.Fatalf("ServeHTTP(single) body = %q, want %q", body, "single-backend")
	}

	reqWildcard := httptest.NewRequest("GET", "http://foo.example.com/", nil)
	reqWildcard.Host = "foo.example.com"
	recWildcard := httptest.NewRecorder()
	engine.Handler().ServeHTTP(recWildcard, reqWildcard)
	if recWildcard.Code != http.StatusOK {
		t.Fatalf("ServeHTTP(wildcard) status = %d, want %d", recWildcard.Code, http.StatusOK)
	}
	if body := recWildcard.Body.String(); body != "wildcard-backend" {
		t.Fatalf("ServeHTTP(wildcard) body = %q, want %q", body, "wildcard-backend")
	}
}

func newBackend(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}
