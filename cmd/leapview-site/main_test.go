package main

import "testing"

func TestParseBaseURL(t *testing.T) {
	for _, test := range []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty for development", raw: ""},
		{name: "HTTPS origin", raw: "https://docs.leapview.dev/", want: "https://docs.leapview.dev"},
		{name: "HTTP origin", raw: "http://localhost:8081", want: "http://localhost:8081"},
		{name: "relative", raw: "/docs", wantErr: true},
		{name: "unsupported scheme", raw: "ftp://docs.leapview.dev", wantErr: true},
		{name: "path", raw: "https://docs.leapview.dev/docs", wantErr: true},
		{name: "query", raw: "https://docs.leapview.dev?preview=1", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseURL, err := parseBaseURL(test.raw)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseBaseURL(%q) succeeded, want error", test.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBaseURL(%q): %v", test.raw, err)
			}
			if baseURL == nil {
				if test.want != "" {
					t.Fatalf("parseBaseURL(%q) = nil, want %q", test.raw, test.want)
				}
				return
			}
			if got := baseURL.String(); got != test.want {
				t.Fatalf("parseBaseURL(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}
