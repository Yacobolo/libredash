package app

import (
	"net/http"

	"github.com/Yacobolo/libredash/internal/agentapp"
	dashboardui "github.com/Yacobolo/libredash/internal/dashboard/ui"
	"github.com/Yacobolo/libredash/internal/ui"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
)

func (s *Server) chatChromeSignal(r *http.Request) ui.ChatSignal {
	return s.chatSignalWith(r.Context(), s.chatScope(r), "", nil, agentapp.ChatArtifactSignals{}, "", false)
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
