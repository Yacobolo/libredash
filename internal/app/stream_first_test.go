package app

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

var dataInitUpdatesPattern = regexp.MustCompile(`data-init="@get\('([^']+)'`)

func renderedWithBootstrap(t *testing.T, server *Server, pageBody, authorization string) string {
	t.Helper()
	return html.UnescapeString(pageBody) + streamBootstrapBody(t, server, pageBody, authorization)
}

func streamBootstrapBody(t *testing.T, server *Server, pageBody, authorization string) string {
	t.Helper()
	decoded := html.UnescapeString(pageBody)
	matches := dataInitUpdatesPattern.FindStringSubmatch(decoded)
	if len(matches) != 2 {
		t.Fatalf("rendered page did not include literal /updates data-init:\n%s", pageBody)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, matches[1], nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "stream-first-test"})
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Routes().ServeHTTP(rec, req)
	}()
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(rec.Body.String(), "datastar-patch-signals") {
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("updates bootstrap did not emit signal patch for %q:\n%s", matches[1], rec.Body.String())
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	body := html.UnescapeString(rec.Body.String())
	for _, forbidden := range []string{`"updatesUrl"`, `"routeKey"`, `"csrfToken"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("updates bootstrap leaked %s for %q:\n%s", forbidden, matches[1], body)
		}
	}
	return body
}
