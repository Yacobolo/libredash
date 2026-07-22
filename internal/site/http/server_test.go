package http_test

import (
	"net/http"
	"testing"

	sitehttp "github.com/Yacobolo/leapview/internal/site/http"
)

func TestNewHandlerReturnsSiteServer(t *testing.T) {
	t.Parallel()

	var handler http.Handler = sitehttp.NewHandler()
	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
}
