package secret

import (
	"crypto/sha256"
	"crypto/subtle"
)

func Equal(got, want string) bool {
	gotDigest := sha256.Sum256([]byte(got))
	wantDigest := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotDigest[:], wantDigest[:]) == 1
}
