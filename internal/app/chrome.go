package app

import (
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/agent"
	dashboardui "github.com/Yacobolo/libredash/internal/dashboard/ui"
	"github.com/Yacobolo/libredash/internal/ui"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
)

func (s *Server) chatChromeSignal(r *http.Request) ui.ChatSignal {
	return s.chatSignalWith(r.Context(), s.chatChromeScope(r), "", nil, agent.ChatArtifactSignals{}, "", false).Agent
}

func (s *Server) chatChromeOption(r *http.Request) ui.ChromeOption {
	return ui.WithChatSidebar(s.chatChromeSignal(r))
}

func (s *Server) dashboardChromeDecorators(r *http.Request) []dashboardui.ChromeDecorator {
	signal := s.chatChromeSignal(r)
	return []dashboardui.ChromeDecorator{
		func(chrome *uisignals.ChromeSignal) {
			uisignals.AttachChatSidebar(&chrome.Sidebar, signal)
		},
	}
}

func (s *Server) chatChromeScope(r *http.Request) agent.Scope {
	principalID := ""
	devBypass := false
	if s.auth != nil {
		if principal, ok := s.auth.Principal(r); ok {
			principalID = principal.ID
			devBypass = principal.DevBypass
		}
	} else if principal, ok := principalFromContext(r.Context()); ok {
		principalID = principal.ID
		devBypass = principal.DevBypass
	}
	return agent.Scope{WorkspaceID: s.chatDefaultWorkspaceID(), PrincipalID: principalID, DevAuthBypass: devBypass}
}

func (s *Server) chatDefaultWorkspaceID() string {
	if strings.TrimSpace(s.defaultWorkspaceID) != "" {
		return s.defaultWorkspaceID
	}
	return s.workspaceID("")
}
