package http

import (
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
)

func TestWriteFilterOptionErrorSuppressesStaleRequests(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	writeFilterOptionError(recorder, dashboardfilter.ErrStaleOptionRequest)

	if recorder.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != `{"filterOptionPages":{}}` {
		t.Fatalf("body = %s, want empty option-page patch", body)
	}
}

func TestWriteFilterOptionErrorKeepsInvalidRequestsAsConflicts(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	writeFilterOptionError(recorder, errors.New("cursor signature is invalid"))

	if recorder.Code != nethttp.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, nethttp.StatusConflict)
	}
}
