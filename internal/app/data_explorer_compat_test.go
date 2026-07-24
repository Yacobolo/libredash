package app

import (
	"net/http"

	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
	workspacehttp "github.com/Yacobolo/leapview/internal/workspace/http"
)

const (
	dataExplorerDefaultLimit = workspacehttp.DataExplorerDefaultLimit
	dataExplorerRowHeight    = workspacehttp.DataExplorerRowHeight
)

func dataExplorerCommandFromQuery(workspaceID, object string) uisignals.DataExplorerCommand {
	return workspacehttp.DataExplorerCommandFromQuery(workspaceID, object)
}

func emptyDataPreviewBlocks(count int, sort uisignals.DataPreviewSortSignal, resetVersion int) map[string]uisignals.DataPreviewBlockSignal {
	return workspacehttp.EmptyDataPreviewBlocks(count, sort, resetVersion)
}

func (s *appTestHarness) globalDataExplorerState(r *http.Request, command uisignals.DataExplorerCommand) (uisignals.DataExplorerPageSignal, uisignals.DataExplorerSignal, error) {
	return s.routes.workspaceModule.HTTP().DataExplorerState(r, command)
}
