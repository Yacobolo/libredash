// Package agent provides a small embedded agent harness for Go applications.
//
// The package owns the model/tool loop, in-memory transcript, bounded tool
// execution, compaction, cancellation, and runtime events. It intentionally
// ships no built-in tools and no concrete model-provider adapter.
package agent
