package staticasset

import (
	"net/url"
	"os"
	"strconv"
	"strings"
)

const versionEnv = "LIBREDASH_ASSET_VERSION"
const generatedVersionPath = "static/asset-version.txt"

func URL(path string) string {
	version := Version()
	return path + "?v=" + url.QueryEscape(version)
}

func Version() string {
	if version := strings.TrimSpace(os.Getenv(versionEnv)); version != "" {
		return version
	}
	if isProduction() {
		if bytes, err := os.ReadFile(generatedVersionPath); err == nil {
			if version := strings.TrimSpace(string(bytes)); version != "" {
				return version
			}
		}
	}
	return "dev"
}

func isProduction() bool {
	value := strings.TrimSpace(os.Getenv("LIBREDASH_PRODUCTION"))
	if value == "" {
		return false
	}
	enabled, err := strconv.ParseBool(value)
	return err == nil && enabled
}
