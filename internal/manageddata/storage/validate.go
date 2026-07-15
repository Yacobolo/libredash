package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func ValidateBlob(blob Blob) error {
	if err := ValidateSHA256(blob.SHA256); err != nil {
		return err
	}
	if blob.Size < 0 {
		return fmt.Errorf("%w: blob size must not be negative", ErrInvalid)
	}
	return nil
}

func ValidateSHA256(value string) error {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return fmt.Errorf("%w: SHA-256 must be 64 lowercase hexadecimal characters", ErrInvalid)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("%w: SHA-256 must be 64 lowercase hexadecimal characters", ErrInvalid)
	}
	return nil
}
