package pagestream

import (
	"regexp"
	"strings"
)

func PostAction(path string, signalPaths ...string) string {
	return requestAction("post", path, signalPaths)
}

func PatchAction(path string, signalPaths ...string) string {
	return requestAction("patch", path, signalPaths)
}

func requestAction(method, path string, signalPaths []string) string {
	options := "headers: window.LibreDashCommand.headers()"
	if len(signalPaths) > 0 {
		patterns := make([]string, 0, len(signalPaths))
		for _, signalPath := range signalPaths {
			patterns = append(patterns, strings.ReplaceAll(regexp.QuoteMeta(signalPath), `\.`, `[.]`))
		}
		include := "/^(?:" + strings.Join(patterns, "|") + ")(?:[.]|$)/"
		options = "filterSignals: {include: " + include + "}, " + options
	}
	return "@" + method + "('" + jsSingleQuoted(path) + "', {" + options + "})"
}
