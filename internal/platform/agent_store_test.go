package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/platform/db"
)

func TestAgentPersistenceLifecycle(t *testing.T) {
	ctx := context.Background()
	store := openAgentTestStore(t, ctx)
	defer store.Close()

	owner := createAgentPrincipal(t, ctx, store, "owner@example.com")
	other := createAgentPrincipal(t, ctx, store, "other@example.com")

	conversation, err := store.CreateAgentConversation(ctx, AgentConversationInput{
		WorkspaceID:  "test",
		PrincipalID:  owner.ID,
		Title:        "Ask about dashboards",
		MetadataJSON: `{"source":"test"}`,
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if conversation.WorkspaceID != "test" || conversation.PrincipalID != owner.ID {
		t.Fatalf("conversation owner = %s/%s, want test/%s", conversation.WorkspaceID, conversation.PrincipalID, owner.ID)
	}
	if conversation.Status != AgentConversationStatusActive {
		t.Fatalf("conversation status = %q, want active", conversation.Status)
	}
	if conversation.TranscriptJson != "[]" {
		t.Fatalf("conversation transcript = %q, want []", conversation.TranscriptJson)
	}
	conversation, err = store.UpdateAgentConversationTranscript(ctx, "test", owner.ID, conversation.ID, `[{"role":"user","content":"seed"}]`)
	if err != nil {
		t.Fatalf("update transcript: %v", err)
	}
	if conversation.TranscriptJson != `[{"role":"user","content":"seed"}]` {
		t.Fatalf("updated transcript = %q", conversation.TranscriptJson)
	}
	defaultConversation, err := store.CreateAgentConversation(ctx, AgentConversationInput{
		WorkspaceID: "test",
		PrincipalID: owner.ID,
	})
	if err != nil {
		t.Fatalf("create default conversation: %v", err)
	}
	defaultConversation, err = store.UpdateDefaultAgentConversationTitle(ctx, "test", owner.ID, defaultConversation.ID, "Available dashboards")
	if err != nil {
		t.Fatalf("update default title: %v", err)
	}
	if defaultConversation.Title != "Available dashboards" {
		t.Fatalf("updated title = %q", defaultConversation.Title)
	}
	if _, err := store.UpdateDefaultAgentConversationTitle(ctx, "test", owner.ID, conversation.ID, "Should not overwrite"); err != sql.ErrNoRows {
		t.Fatalf("update non-default title error = %v, want sql.ErrNoRows", err)
	}
	if _, err := store.ArchiveAgentConversation(ctx, "test", owner.ID, defaultConversation.ID); err != nil {
		t.Fatalf("archive default conversation: %v", err)
	}

	hidden, err := store.CreateAgentConversation(ctx, AgentConversationInput{
		WorkspaceID: "test",
		PrincipalID: other.ID,
		Title:       "Other user chat",
	})
	if err != nil {
		t.Fatalf("create hidden conversation: %v", err)
	}
	conversations, err := store.ListAgentConversations(ctx, "test", owner.ID)
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 || conversations[0].ID != conversation.ID {
		t.Fatalf("visible conversations = %#v, want only %s", conversations, conversation.ID)
	}
	if _, err := store.GetAgentConversation(ctx, "test", owner.ID, hidden.ID); err != sql.ErrNoRows {
		t.Fatalf("get other principal conversation error = %v, want sql.ErrNoRows", err)
	}

	userMessage, err := store.AppendAgentMessage(ctx, AgentMessageInput{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: conversation.ID,
		Role:           AgentMessageRoleUser,
		ContentText:    "What dashboards can I use?",
		ContentJSON:    `{"text":"What dashboards can I use?"}`,
		RunID:          "",
		ToolCallID:     "",
		ToolName:       "",
	})
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	assistantMessage, err := store.AppendAgentMessage(ctx, AgentMessageInput{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: conversation.ID,
		Role:           AgentMessageRoleAssistant,
		ContentText:    "You can use Executive Sales.",
		ContentJSON:    `{}`,
	})
	if err != nil {
		t.Fatalf("append assistant message: %v", err)
	}
	if userMessage.Seq != 1 || assistantMessage.Seq != 2 {
		t.Fatalf("message seqs = %d,%d, want 1,2", userMessage.Seq, assistantMessage.Seq)
	}
	messages, err := store.ListAgentMessages(ctx, "test", owner.ID, conversation.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != AgentMessageRoleUser || messages[1].Role != AgentMessageRoleAssistant {
		t.Fatalf("messages = %#v", messages)
	}
	if _, err := store.ListAgentMessages(ctx, "test", other.ID, conversation.ID); err != sql.ErrNoRows {
		t.Fatalf("list other principal messages error = %v, want sql.ErrNoRows", err)
	}

	run, err := store.CreateAgentRun(ctx, AgentRunInput{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: conversation.ID,
		RunID:          "run_external",
		Model:          "gpt-test",
		MetadataJSON:   `{"provider":"fake"}`,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.Status != AgentRunStatusRunning {
		t.Fatalf("run status = %q, want running", run.Status)
	}
	if run.ID != "run_external" {
		t.Fatalf("run id = %q, want explicit id", run.ID)
	}
	eventOne, err := store.AppendAgentEvent(ctx, AgentEventInput{
		WorkspaceID: "test",
		PrincipalID: owner.ID,
		RunID:       run.ID,
		Sequence:    7,
		EventType:   "model_request",
		Severity:    "debug",
		PayloadJSON: `{"purpose":"turn"}`,
	})
	if err != nil {
		t.Fatalf("append event one: %v", err)
	}
	eventTwo, err := store.AppendAgentEvent(ctx, AgentEventInput{
		WorkspaceID: "test",
		PrincipalID: owner.ID,
		RunID:       run.ID,
		Sequence:    8,
		EventType:   "model_response",
		Severity:    "debug",
		PayloadJSON: `{"finish":"stop"}`,
	})
	if err != nil {
		t.Fatalf("append event two: %v", err)
	}
	if eventOne.Seq != 7 || eventTwo.Seq != 8 {
		t.Fatalf("event seqs = %d,%d, want 7,8", eventOne.Seq, eventTwo.Seq)
	}
	run, err = store.FinishAgentRun(ctx, AgentRunFinish{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: conversation.ID,
		RunID:          run.ID,
		Status:         AgentRunStatusCompleted,
		StopReason:     "completed",
		InputTokens:    10,
		OutputTokens:   20,
		TotalTokens:    30,
		MetadataJSON:   `{"provider":"fake","model":"gpt-test"}`,
	})
	if err != nil {
		t.Fatalf("finish run: %v", err)
	}
	if run.Status != AgentRunStatusCompleted || run.TotalTokens != 30 || !run.FinishedAt.Valid {
		t.Fatalf("finished run = %#v", run)
	}
	runs, err := store.ListAgentRuns(ctx, "test", owner.ID, conversation.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs = %#v, want %s", runs, run.ID)
	}
	events, err := store.ListAgentEvents(ctx, "test", owner.ID, run.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 || events[0].Seq != 7 || events[1].Seq != 8 {
		t.Fatalf("events = %#v", events)
	}

	archived, err := store.ArchiveAgentConversation(ctx, "test", owner.ID, conversation.ID)
	if err != nil {
		t.Fatalf("archive conversation: %v", err)
	}
	if archived.Status != AgentConversationStatusArchived || !archived.ArchivedAt.Valid {
		t.Fatalf("archived conversation = %#v", archived)
	}
	conversations, err = store.ListAgentConversations(ctx, "test", owner.ID)
	if err != nil {
		t.Fatalf("list after archive: %v", err)
	}
	if len(conversations) != 0 {
		t.Fatalf("active conversations after archive = %#v, want none", conversations)
	}
}

func TestAgentMigrationCreatesTablesAndIndexes(t *testing.T) {
	ctx := context.Background()
	store := openAgentTestStore(t, ctx)
	defer store.Close()

	for _, name := range []string{
		"agent_conversations",
		"agent_messages",
		"agent_runs",
		"agent_events",
		"agent_conversations_owner_updated_idx",
		"agent_messages_conversation_seq_idx",
		"agent_runs_conversation_started_idx",
		"agent_events_run_seq_idx",
	} {
		t.Run(name, func(t *testing.T) {
			var count int
			if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE name = ?`, name).Scan(&count); err != nil {
				t.Fatalf("query sqlite_master: %v", err)
			}
			if count != 1 {
				t.Fatalf("sqlite object %q count = %d, want 1", name, count)
			}
		})
	}
}

func TestAgentPersistenceRejectsInvalidJSON(t *testing.T) {
	ctx := context.Background()
	store := openAgentTestStore(t, ctx)
	defer store.Close()
	principal := createAgentPrincipal(t, ctx, store, "owner@example.com")

	if _, err := store.CreateAgentConversation(ctx, AgentConversationInput{
		WorkspaceID:  "test",
		PrincipalID:  principal.ID,
		Title:        "Bad metadata",
		MetadataJSON: `{`,
	}); err == nil {
		t.Fatal("CreateAgentConversation accepted invalid metadata JSON")
	}

	conversation, err := store.CreateAgentConversation(ctx, AgentConversationInput{
		WorkspaceID: "test",
		PrincipalID: principal.ID,
		Title:       "Good chat",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, AgentMessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           AgentMessageRoleUser,
		ContentJSON:    `{`,
	}); err == nil {
		t.Fatal("AppendAgentMessage accepted invalid content JSON")
	}
	if _, err := store.UpdateAgentConversationTranscript(ctx, "test", principal.ID, conversation.ID, `{}`); err == nil {
		t.Fatal("UpdateAgentConversationTranscript accepted non-array transcript JSON")
	}
	run, err := store.CreateAgentRun(ctx, AgentRunInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Model:          "gpt-test",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := store.AppendAgentEvent(ctx, AgentEventInput{
		WorkspaceID: "test",
		PrincipalID: principal.ID,
		RunID:       run.ID,
		Sequence:    1,
		EventType:   "model_request",
		Severity:    "debug",
		PayloadJSON: `{`,
	}); err == nil {
		t.Fatal("AppendAgentEvent accepted invalid payload JSON")
	}
}

func TestAgentPersistenceCascadesConversationDelete(t *testing.T) {
	ctx := context.Background()
	store := openAgentTestStore(t, ctx)
	defer store.Close()
	principal := createAgentPrincipal(t, ctx, store, "owner@example.com")
	conversation, err := store.CreateAgentConversation(ctx, AgentConversationInput{
		WorkspaceID: "test",
		PrincipalID: principal.ID,
		Title:       "Cascade chat",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, AgentMessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           AgentMessageRoleUser,
		ContentText:    "hello",
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}
	run, err := store.CreateAgentRun(ctx, AgentRunInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Model:          "gpt-test",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := store.AppendAgentEvent(ctx, AgentEventInput{
		WorkspaceID: "test",
		PrincipalID: principal.ID,
		RunID:       run.ID,
		Sequence:    1,
		EventType:   "model_request",
		Severity:    "debug",
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `DELETE FROM agent_conversations WHERE id = ?`, conversation.ID); err != nil {
		t.Fatalf("delete conversation: %v", err)
	}
	for table, column := range map[string]string{
		"agent_messages": "conversation_id",
		"agent_runs":     "conversation_id",
		"agent_events":   "run_id",
	} {
		var count int
		query := `SELECT count(*) FROM ` + table + ` WHERE ` + column + ` = ?`
		arg := conversation.ID
		if table == "agent_events" {
			arg = run.ID
		}
		if err := store.db.QueryRowContext(ctx, query, arg).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows after cascade = %d, want 0", table, count)
		}
	}
}

func openAgentTestStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.EnsureWorkspace(ctx, WorkspaceInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	return store
}

func createAgentPrincipal(t *testing.T, ctx context.Context, store *Store, email string) db.Principal {
	t.Helper()
	principal, err := store.UpsertPrincipal(ctx, PrincipalInput{ID: PrincipalIDForEmail(email), Email: email, DisplayName: email})
	if err != nil {
		t.Fatalf("upsert principal %s: %v", email, err)
	}
	if err := store.BindRole(ctx, "test", principal.ID, "viewer"); err != nil {
		t.Fatalf("bind role: %v", err)
	}
	return principal
}

func assertJSON(t *testing.T, raw string) {
	t.Helper()
	if !json.Valid([]byte(raw)) {
		t.Fatalf("invalid JSON: %s", raw)
	}
}
