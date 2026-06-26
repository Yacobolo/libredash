package ui

import (
	"github.com/Yacobolo/libredash/internal/agentapp"
	workspaceview "github.com/Yacobolo/libredash/internal/workspace"
)

type WorkspaceAccessResponse struct {
	Workspace workspaceview.WorkspaceView     `json:"workspace"`
	Roles     []workspaceview.RoleView        `json:"roles"`
	Bindings  []workspaceview.RoleBindingView `json:"bindings"`
	CanManage bool                            `json:"canManage"`
	Status    WorkspaceAccessStatus           `json:"status"`
}

type WorkspaceAccessStatus struct {
	Loading bool   `json:"loading"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

type WorkspaceAccessCommand struct {
	Email       string `json:"email"`
	Role        string `json:"role"`
	PrincipalID string `json:"principalId"`
}

type ChatSignal struct {
	Conversations        []ChatConversationSummary     `json:"conversations"`
	ActiveConversationID string                        `json:"activeConversationId"`
	Transcript           []agentapp.ChatTranscriptItem `json:"transcript"`
	Status               ChatStatus                    `json:"status"`
	Composer             ComposerSignal                `json:"composer"`
}

type ChatConversationSummary struct {
	ID              string `json:"id"`
	WorkspaceID     string `json:"workspaceId"`
	PrincipalID     string `json:"principalId"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	MessageCount    int    `json:"messageCount"`
	LastMessageText string `json:"lastMessageText,omitempty"`
	TitlePending    bool   `json:"titlePending,omitempty"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	ArchivedAt      string `json:"archivedAt,omitempty"`
}

type ChatStatus struct {
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

type ComposerSignal struct {
	Value       string `json:"value"`
	Disabled    bool   `json:"disabled"`
	Placeholder string `json:"placeholder"`
}
