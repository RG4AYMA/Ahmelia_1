package gateway

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func hostOf(t *testing.T, raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func TestWhitelistedHostCannotRedirectToUnlistedHost(t *testing.T) {
	evil := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("evil")) }))
	defer evil.Close()
	good := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/x", http.StatusFound)
	}))
	defer good.Close()
	g := NewGatewayWithSources([]AuthorizedSource{{Name: "good", Host: hostOf(t, good.URL)}})
	g.Client = good.Client()
	g.Retries = 0
	if _, err := g.Fetch(good.URL + "/doc"); err == nil || !strings.Contains(err.Error(), "bloqueada") {
		t.Fatalf("redirect to unlisted host was followed: %v", err)
	}
}

func TestFetchCapturesProvenanceAndFlagsTruncation(t *testing.T) {
	body := strings.Repeat("a", MaxBodyBytes+10)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Mon, 12 Oct 2026 10:00:00 GMT")
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	g := NewGatewayWithSources([]AuthorizedSource{{Name: "t", Host: hostOf(t, srv.URL)}})
	g.Client = srv.Client()
	g.Retries = 0
	d, err := g.Fetch(srv.URL + "/big")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Truncated || len(d.ContentSHA256) != 64 || d.LastModified == "" || d.ETag != `"v1"` || d.Host == "" {
		t.Fatalf("%+v", d)
	}
}
