package access

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type retryAuditRecorder struct {
	failures int
	calls    int
}

func (r *retryAuditRecorder) RecordAuditEvent(context.Context, AuditEventInput) error {
	r.calls++
	if r.failures > 0 {
		r.failures--
		return errors.New("database busy")
	}
	return nil
}

func TestPersistAuditEventRetriesTransientFailure(t *testing.T) {
	recorder := &retryAuditRecorder{failures: 2}
	if err := PersistAuditEvent(t.Context(), recorder, AuditEventInput{Action: "grant.created"}); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 3 {
		t.Fatalf("calls = %d, want 3", recorder.calls)
	}
}

func TestPersistAuditEventReturnsPersistentFailure(t *testing.T) {
	recorder := &retryAuditRecorder{failures: auditWriteAttempts}
	err := PersistAuditEvent(t.Context(), recorder, AuditEventInput{Action: "grant.created"})
	if err == nil || !strings.Contains(err.Error(), "database busy") {
		t.Fatalf("error = %v, want persistent storage error", err)
	}
	if recorder.calls != auditWriteAttempts {
		t.Fatalf("calls = %d, want %d", recorder.calls, auditWriteAttempts)
	}
}
