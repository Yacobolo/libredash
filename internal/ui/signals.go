package ui

import (
	"github.com/Yacobolo/libredash/internal/agent"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
)

type WorkspaceAccessResponse = uisignals.WorkspaceAccessResponse
type WorkspaceAccessStatus = uisignals.WorkspaceAccessStatus
type WorkspaceAccessCommand = uisignals.WorkspaceAccessCommand
type ChatSignal = uisignals.ChatSignal
type ChatViewState = uisignals.ChatViewState
type ChatConversationSummary = uisignals.ChatConversationSummary
type ChatStatus = uisignals.ChatStatus
type ComposerSignal = uisignals.ComposerSignal
type ChatTranscriptItemSignal = uisignals.ChatTranscriptItemSignal

func ChatTranscriptItems(items []agent.ChatTranscriptItem) []ChatTranscriptItemSignal {
	return uisignals.ChatTranscriptItems(items)
}
