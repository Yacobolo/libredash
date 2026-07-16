package staticasset

import (
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/Yacobolo/libredash/internal/configspec"
)

const versionEnv = configspec.EnvLIBREDASH_ASSET_VERSION
const generatedVersionPath = "static/asset-version.txt"

const DatastarScriptPath = "/static/vendor/datastar-1.0.2.js"

func URL(path string) string {
	version := Version()
	return path + "?v=" + url.QueryEscape(version)
}

func Version() string {
	if version := strings.TrimSpace(os.Getenv(versionEnv)); version != "" {
		return version
	}
	if Production() {
		if bytes, err := os.ReadFile(generatedVersionPath); err == nil {
			if version := strings.TrimSpace(string(bytes)); version != "" {
				return version
			}
		}
	}
	return "dev"
}

func Production() bool {
	value := strings.TrimSpace(os.Getenv(configspec.EnvLIBREDASH_PRODUCTION))
	if value == "" {
		return false
	}
	enabled, err := strconv.ParseBool(value)
	return err == nil && enabled
}
