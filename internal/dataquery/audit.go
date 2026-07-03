package dataquery

import (
	"context"
	"errors"
	"strings"
	"time"
)

type AuditRecorder interface {
	RecordDataQuery(ctx context.Context, query Query, result Result) error
}

var ErrMissingPrincipal = errors.New("data query requires principal id")

type auditRecorderContextKey struct{}
type auditRecordedContextKey struct{}

func WithAuditRecorder(ctx context.Context, recorder AuditRecorder) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, auditRecorderContextKey{}, recorder)
}

func AuditRecorderFromContext(ctx context.Context) (AuditRecorder, bool) {
	recorder, ok := ctx.Value(auditRecorderContextKey{}).(AuditRecorder)
	return recorder, ok && recorder != nil
}

func ExecuteAudited(ctx context.Context, request Query, execute func(context.Context, Query) (Result, error)) (Result, error) {
	request = request.WithMetadata(MetadataFromContext(ctx))
	_, hasRecorder := AuditRecorderFromContext(ctx)
	if hasRecorder && strings.TrimSpace(request.PrincipalID) == "" {
		return queryResultForError(ctx, Result{}, ErrMissingPrincipal, 0), ErrMissingPrincipal
	}
	if err := request.Validate(); err != nil {
		result := queryResultForError(ctx, Result{}, err, 0)
		recordDataQuery(ctx, request, result)
		return result, err
	}
	if _, recorded := ctx.Value(auditRecordedContextKey{}).(bool); recorded {
		return execute(ctx, request)
	}
	start := time.Now()
	ctx = context.WithValue(ctx, auditRecordedContextKey{}, true)
	result, err := execute(ctx, request)
	result = queryResultForError(ctx, result, err, time.Since(start).Milliseconds())
	if err == nil {
		result.Status = firstNonEmpty(result.Status, StatusSuccess)
	}
	recordDataQuery(ctx, request, result)
	return result, err
}

func recordDataQuery(ctx context.Context, request Query, result Result) {
	recorder, ok := AuditRecorderFromContext(ctx)
	if !ok {
		return
	}
	_ = recorder.RecordDataQuery(ctx, request, result)
}

func queryResultForError(ctx context.Context, result Result, err error, durationMS int64) Result {
	if durationMS > 0 && result.DurationMS == 0 {
		result.DurationMS = durationMS
	}
	if result.RowsReturned == 0 && len(result.Rows) > 0 {
		result.RowsReturned = len(result.Rows)
	}
	if err != nil {
		result.Status = queryStatus(ctx, err)
		if result.ExecutionState == "" {
			result.ExecutionState = queryExecutionState(ctx, err)
		}
		result.Error = sanitizeQueryError(err)
		return result
	}
	return result
}

func queryStatus(ctx context.Context, err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return StatusCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return StatusTimeout
	}
	return StatusError
}

func sanitizeQueryError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	return message
}

func queryExecutionState(ctx context.Context, err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return ExecutionCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ExecutionTimeout
	}
	return ExecutionFailed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
