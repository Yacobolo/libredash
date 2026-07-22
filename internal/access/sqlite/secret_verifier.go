package sqlite

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	"github.com/Yacobolo/leapview/internal/configspec"
	"github.com/alexedwards/argon2id"
)

var secretVerifierParams = &argon2id.Params{
	Memory:      19 * 1024,
	Iterations:  2,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

func secretFingerprint(secret string) string {
	mac := hmac.New(sha256.New, tokenFingerprintKey)
	mac.Write([]byte(secret))
	return hex.EncodeToString(mac.Sum(nil))
}

func newSecretVerifier(secret string) (string, error) {
	return argon2id.CreateHash(secret, secretVerifierParams)
}

func verifySecret(secret, verifier string) bool {
	match, err := argon2id.ComparePasswordAndHash(secret, verifier)
	return err == nil && match
}

var tokenFingerprintKey = func() []byte {
	source := firstNonEmpty(
		strings.TrimSpace(os.Getenv(configspec.EnvLEAPVIEW_TOKEN_HASH_KEY)),
		strings.TrimSpace(os.Getenv(configspec.EnvLEAPVIEW_CSRF_KEY)),
		"leapview-development-token-hash-key",
	)
	sum := sha256.Sum256([]byte("leapview-token-fingerprint:" + source))
	return sum[:]
}()
