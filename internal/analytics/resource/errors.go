package resource

import (
	"errors"
	"fmt"
	"strings"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
)

type ResourceExhaustedReason string

const (
	ResourceMemory           ResourceExhaustedReason = "memory"
	ResourceTemporaryStorage ResourceExhaustedReason = "temp"
)

type ResourceExhaustedError struct {
	Reason ResourceExhaustedReason
	Err    error
}

func (e *ResourceExhaustedError) Error() string {
	return fmt.Sprintf("analytical %s resource exhausted", e.Reason)
}
func (e *ResourceExhaustedError) Unwrap() error { return e.Err }
func ResourceExhaustedReasonOf(err error) (ResourceExhaustedReason, bool) {
	var target *ResourceExhaustedError
	if !errors.As(err, &target) {
		return "", false
	}
	return target.Reason, true
}

func Classify(err error) error {
	if err == nil {
		return nil
	}
	var driverErr *duckdbdriver.Error
	if !errors.As(err, &driverErr) || driverErr.Type != duckdbdriver.ErrorTypeOutOfMemory {
		return err
	}
	reason := ResourceMemory
	text := strings.ToLower(driverErr.Error())
	if strings.Contains(text, "temp") || strings.Contains(text, "spill") {
		reason = ResourceTemporaryStorage
	}
	return &ResourceExhaustedError{Reason: reason, Err: err}
}
