package app

import (
	"net/http"

	"github.com/Yacobolo/leapview/internal/agent"
	dashboardui "github.com/Yacobolo/leapview/internal/dashboard/ui"
	"github.com/Yacobolo/leapview/internal/ui"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
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
	return agent.Scope{PrincipalID: principalID, DevAuthBypass: devBypass}
}
