package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/agent"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestRepositoryPersistsConversationRunMessagesAndEvents(t *testing.T) {
	ctx := context.Background()
	store, repo := openAgentRepo(t, ctx)
	owner := createAgentPrincipal(t, ctx, store, "owner@example.com")
	other := createAgentPrincipal(t, ctx, store, "other@example.com")

	conversation, err := repo.CreateConversation(ctx, agent.ConversationInput{
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
	if conversation.Status != agent.ConversationStatusActive || conversation.TranscriptJSON != "[]" {
		t.Fatalf("conversation = %#v", conversation)
	}
	conversation, err = repo.UpdateConversationTranscript(ctx, "test", owner.ID, conversation.ID, `[{"role":"user","content":"seed"}]`)
	if err != nil {
		t.Fatalf("update transcript: %v", err)
	}
	if conversation.TranscriptJSON != `[{"role":"user","content":"seed"}]` {
		t.Fatalf("updated transcript = %q", conversation.TranscriptJSON)
	}

	hidden, err := repo.CreateConversation(ctx, agent.ConversationInput{
		WorkspaceID: "test",
		PrincipalID: other.ID,
		Title:       "Other user chat",
	})
	if err != nil {
		t.Fatalf("create hidden conversation: %v", err)
	}
	conversations, err := repo.ListConversations(ctx, "test", owner.ID)
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 || conversations[0].ID != conversation.ID {
		t.Fatalf("visible conversations = %#v, want only %s", conversations, conversation.ID)
	}
	if _, err := repo.GetConversation(ctx, "test", owner.ID, hidden.ID); err != sql.ErrNoRows {
		t.Fatalf("get other principal conversation error = %v, want sql.ErrNoRows", err)
	}
	conversation, err = repo.UpdateConversation(ctx, agent.ConversationUpdate{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: conversation.ID,
		Title:          "Updated title",
	})
	if err != nil {
		t.Fatalf("update conversation: %v", err)
	}
	if conversation.Title != "Updated title" {
		t.Fatalf("updated title = %q", conversation.Title)
	}

	userMessage, err := repo.AppendMessage(ctx, agent.MessageInput{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: conversation.ID,
		Role:           agent.MessageRoleUser,
		ContentText:    "What dashboards can I use?",
		ContentJSON:    `{"text":"What dashboards can I use?"}`,
	})
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	assistantMessage, err := repo.AppendMessage(ctx, agent.MessageInput{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: conversation.ID,
		Role:           agent.MessageRoleAssistant,
		ContentText:    "You can use Executive Sales.",
	})
	if err != nil {
		t.Fatalf("append assistant message: %v", err)
	}
	if userMessage.Seq != 1 || assistantMessage.Seq != 2 {
		t.Fatalf("message seqs = %d,%d, want 1,2", userMessage.Seq, assistantMessage.Seq)
	}
	messages, err := repo.ListMessages(ctx, "test", owner.ID, conversation.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != agent.MessageRoleUser || messages[1].Role != agent.MessageRoleAssistant {
		t.Fatalf("messages = %#v", messages)
	}
	if _, err := repo.ListMessages(ctx, "test", other.ID, conversation.ID); err != sql.ErrNoRows {
		t.Fatalf("list other principal messages error = %v, want sql.ErrNoRows", err)
	}

	run, err := repo.CreateRun(ctx, agent.RunInput{
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
	if run.Status != agent.RunStatusRunning || run.ID != "run_external" {
		t.Fatalf("run = %#v", run)
	}
	eventOne, err := repo.AppendEvent(ctx, agent.EventInput{
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
	eventTwo, err := repo.AppendEvent(ctx, agent.EventInput{
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
	run, err = repo.FinishRun(ctx, agent.RunFinish{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: conversation.ID,
		RunID:          run.ID,
		Status:         agent.RunStatusCompleted,
		StopReason:     "completed",
		InputTokens:    10,
		OutputTokens:   20,
		TotalTokens:    30,
		MetadataJSON:   `{"provider":"fake","model":"gpt-test"}`,
	})
	if err != nil {
		t.Fatalf("finish run: %v", err)
	}
	if run.Status != agent.RunStatusCompleted {
		t.Fatalf("finished run = %#v", run)
	}
	gotRun, err := repo.GetRun(ctx, "test", owner.ID, conversation.ID, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if gotRun.ID != run.ID || gotRun.ConversationID != conversation.ID || gotRun.TotalTokens != 30 || gotRun.FinishedAt == "" {
		t.Fatalf("got run = %#v", gotRun)
	}
	runs, err := repo.ListRunsPage(ctx, "test", owner.ID, conversation.ID, agent.Page{Limit: 1})
	if err != nil {
		t.Fatalf("list runs page: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs page = %#v", runs)
	}
	events, err := repo.ListEvents(ctx, "test", owner.ID, run.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 || events[0].Seq != 7 || events[1].Seq != 8 {
		t.Fatalf("events = %#v", events)
	}
	events, err = repo.ListEventsPage(ctx, "test", owner.ID, run.ID, agent.Page{Limit: 1, After: eventOne.ID})
	if err != nil {
		t.Fatalf("list events page: %v", err)
	}
	if len(events) != 1 || events[0].ID != eventTwo.ID {
		t.Fatalf("events page = %#v", events)
	}
	archived, err := repo.ArchiveConversation(ctx, "test", owner.ID, conversation.ID)
	if err != nil {
		t.Fatalf("archive conversation: %v", err)
	}
	if archived.Status != agent.ConversationStatusArchived || archived.ArchivedAt == "" {
		t.Fatalf("archived conversation = %#v", archived)
	}
	conversations, err = repo.ListConversations(ctx, "test", owner.ID)
	if err != nil {
		t.Fatalf("list after archive: %v", err)
	}
	if len(conversations) != 0 {
		t.Fatalf("archived conversation should be hidden from active list: %#v", conversations)
	}
}

func TestRepositoryScopesConversationsToPrincipalNotWorkspace(t *testing.T) {
	ctx := context.Background()
	store, repo := openAgentRepo(t, ctx)
	owner := createAgentPrincipal(t, ctx, store, "global-owner@example.com")

	conversation, err := repo.CreateConversation(ctx, agent.ConversationInput{
		WorkspaceID: "sales",
		PrincipalID: owner.ID,
		Title:       "Global conversation",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	conversations, err := repo.ListConversations(ctx, "operations", owner.ID)
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 || conversations[0].ID != conversation.ID {
		t.Fatalf("conversations = %#v, want principal-owned conversation %s", conversations, conversation.ID)
	}
	if _, err := repo.GetConversation(ctx, "operations", owner.ID, conversation.ID); err != nil {
		t.Fatalf("get conversation through another workspace scope: %v", err)
	}
}

func TestRepositoryRejectsInvalidJSON(t *testing.T) {
	ctx := context.Background()
	store, repo := openAgentRepo(t, ctx)
	principal := createAgentPrincipal(t, ctx, store, "owner@example.com")

	if _, err := repo.CreateConversation(ctx, agent.ConversationInput{
		WorkspaceID:  "test",
		PrincipalID:  principal.ID,
		Title:        "Bad metadata",
		MetadataJSON: `{`,
	}); err == nil {
		t.Fatal("CreateConversation accepted invalid metadata JSON")
	}
	conversation, err := repo.CreateConversation(ctx, agent.ConversationInput{
		WorkspaceID: "test",
		PrincipalID: principal.ID,
		Title:       "Good chat",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := repo.AppendMessage(ctx, agent.MessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           agent.MessageRoleUser,
		ContentJSON:    `{`,
	}); err == nil {
		t.Fatal("AppendMessage accepted invalid content JSON")
	}
	if _, err := repo.UpdateConversationTranscript(ctx, "test", principal.ID, conversation.ID, `{}`); err == nil {
		t.Fatal("UpdateConversationTranscript accepted non-array transcript JSON")
	}
}

func openAgentRepo(t *testing.T, ctx context.Context) (*platform.Store, *Repository) {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	return store, NewRepository(store.SQLDB())
}

func createAgentPrincipal(t *testing.T, ctx context.Context, store *platform.Store, email string) access.Principal {
	t.Helper()
	repo := accesssqlite.NewRepository(store.SQLDB())
	principal, err := repo.SetPrincipalRole(ctx, access.PrincipalRoleInput{WorkspaceID: "test", Email: email, DisplayName: email, Role: "viewer"})
	if err != nil {
		t.Fatalf("set principal role %s: %v", email, err)
	}
	return principal
}
