package pagestream

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnsureClientIDKeepsExistingCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: defaultClientIDCookieName, Value: "client-1"})
	rec := httptest.NewRecorder()

	if got := EnsureClientID(rec, req); got != "client-1" {
		t.Fatalf("client id = %q", got)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("unexpected replacement cookie: %#v", cookies)
	}
}

func TestEnsureClientIDSetsMissingCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	clientID := EnsureClientID(rec, req)
	if clientID == "" {
		t.Fatal("client id is empty")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v, want one client id cookie", cookies)
	}
	if cookies[0].Name != defaultClientIDCookieName || cookies[0].Value != clientID || cookies[0].Path != "/" || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("client id cookie = %#v", cookies[0])
	}
}

func TestClientIDFromRequestPrefersSignalThenCookieThenDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: defaultClientIDCookieName, Value: "cookie-client"})

	if got := ClientIDFromRequest(req, "signal-client"); got != "signal-client" {
		t.Fatalf("client id from signal = %q", got)
	}
	if got := ClientIDFromRequest(req, ""); got != "cookie-client" {
		t.Fatalf("client id from cookie = %q", got)
	}
	if got := ClientIDFromRequest(httptest.NewRequest(http.MethodGet, "/", nil), ""); got != "default" {
		t.Fatalf("client id default = %q", got)
	}
}
