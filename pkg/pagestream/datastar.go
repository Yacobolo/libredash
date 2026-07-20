package pagestream

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	ds "github.com/starfederation/datastar-go/datastar"
)

const defaultClientIDCookieName = "lv_client_id"

func ReadSignals(r *http.Request, target any) error {
	return ds.ReadSignals(r, target)
}

func EnsureClientID(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(defaultClientIDCookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	clientID := newClientID()
	http.SetCookie(w, &http.Cookie{
		Name:     defaultClientIDCookieName,
		Value:    clientID,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	})
	return clientID
}

func ClientIDFromRequest(r *http.Request, signalClientID string) string {
	if signalClientID != "" {
		return signalClientID
	}
	cookie, err := r.Cookie(defaultClientIDCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return "default"
}

func newClientID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(bytes[:])
}
