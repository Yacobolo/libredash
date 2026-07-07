package queryaudit

import "regexp"

const redactedValue = "[REDACTED]"

var (
	sensitiveSQLSingleQuotedValuePattern = regexp.MustCompile(`(?i)\b(password|passwd|secret|token|client_secret|secret_access_key|access_token|refresh_token|api_key|authorization)\b(\s*(?:=>|=)?\s*)'((?:''|[^'])*)'`)
	sensitiveJSONQuotedValuePattern     = regexp.MustCompile(`(?i)("(?:(?:api[_-]?key)|(?:client[_-]?secret)|(?:secret[_-]?access[_-]?key)|(?:access[_-]?token)|(?:refresh[_-]?token)|(?:password)|(?:passwd)|(?:token)|(?:secret)|(?:authorization))"\s*:\s*)"((?:\\.|[^"\\])*)"`)
	urlPasswordPattern                   = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)([^/?#'"\s@]*:)([^/?#'"\s@]+)@`)
	urlTokenPattern                      = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)([^/?#'"\s@:]+)@`)
)

func RedactSensitiveText(text string) string {
	if text == "" {
		return ""
	}
	text = urlPasswordPattern.ReplaceAllString(text, "$1$2"+redactedValue+"@")
	text = urlTokenPattern.ReplaceAllString(text, "$1"+redactedValue+"@")
	text = sensitiveSQLSingleQuotedValuePattern.ReplaceAllString(text, "$1$2'"+redactedValue+"'")
	text = sensitiveJSONQuotedValuePattern.ReplaceAllString(text, "$1\""+redactedValue+"\"")
	return text
}
