package actions

import (
	"regexp"
	"strings"
)

func Post(path string, signalPaths ...string) string {
	return request("post", path, signalPaths)
}

func Patch(path string, signalPaths ...string) string {
	return request("patch", path, signalPaths)
}

func request(method, path string, signalPaths []string) string {
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

func jsSingleQuoted(value string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`, "\r", `\r`).Replace(value)
}
