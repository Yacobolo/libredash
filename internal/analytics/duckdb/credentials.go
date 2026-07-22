package duckdb

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Yacobolo/leapview/internal/analytics/connectors"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
)

// CredentialResolver is an infrastructure boundary. Authored and compiled
// model values contain only references; resolved secret values exist only for
// the lifetime of an admitted refresh preparation.
type CredentialResolver interface {
	Resolve(context.Context, string, semanticmodel.Connection) (semanticmodel.ConnectionAuth, error)
}

type EnvironmentCredentialResolver struct{}

func (EnvironmentCredentialResolver) Resolve(_ context.Context, name string, connection semanticmodel.Connection) (semanticmodel.ConnectionAuth, error) {
	provider := strings.TrimSpace(connection.Credentials.Provider)
	switch provider {
	case "", "none":
		return nil, nil
	case "ambient":
		auth := semanticmodel.ConnectionAuth{}
		if connection.Credentials.Region != "" {
			auth["region"] = connection.Credentials.Region
		}
		if connection.Credentials.Endpoint != "" {
			auth["endpoint"] = connection.Credentials.Endpoint
		}
		if connection.Credentials.AccountName != "" {
			auth["account_name"] = connection.Credentials.AccountName
		}
		return auth, nil
	case "env":
		secretName := strings.TrimSpace(connection.Credentials.Secret)
		value, ok := os.LookupEnv(secretName)
		if !ok {
			return nil, fmt.Errorf("connection %q credential reference is unavailable", name)
		}
		var object map[string]any
		if err := json.Unmarshal([]byte(value), &object); err == nil {
			return semanticmodel.ConnectionAuth(object), nil
		}
		spec, ok := connectors.LookupConnection(connection.Kind)
		if !ok {
			return nil, fmt.Errorf("connection %q has unsupported kind %q", name, connection.Kind)
		}
		for _, key := range []string{"connection_string", "token"} {
			if containsString(spec.AuthKeys, key) {
				return semanticmodel.ConnectionAuth{key: value}, nil
			}
		}
		return nil, fmt.Errorf("connection %q credential reference has an invalid shape", name)
	default:
		return nil, fmt.Errorf("connection %q has unsupported credential provider %q", name, provider)
	}
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
