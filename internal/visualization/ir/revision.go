package ir

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const specRevisionPrefix = "sha256:"

// SpecRevision is the content address of a canonical generated visualization
// specification. It is deliberately distinct from runtime data revisions.
type SpecRevision string

// ComputeSpecRevision hashes the generated JSON representation of a typed
// specification. Generated specifications contain no arbitrary option maps, so
// their encoding is deterministic for semantically identical values.
func ComputeSpecRevision(spec VisualizationSpec) (SpecRevision, error) {
	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshal visualization specification: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return SpecRevision(specRevisionPrefix + hex.EncodeToString(digest[:])), nil
}

// ParseSpecRevision validates the canonical external representation.
func ParseSpecRevision(value string) (SpecRevision, error) {
	hexDigest, ok := strings.CutPrefix(value, specRevisionPrefix)
	if !ok || len(hexDigest) != sha256.Size*2 {
		return "", fmt.Errorf("invalid visualization specification revision %q", value)
	}
	digest, err := hex.DecodeString(hexDigest)
	if err != nil || hex.EncodeToString(digest) != hexDigest {
		return "", fmt.Errorf("invalid visualization specification revision %q", value)
	}
	return SpecRevision(value), nil
}

func (revision SpecRevision) String() string {
	return string(revision)
}
