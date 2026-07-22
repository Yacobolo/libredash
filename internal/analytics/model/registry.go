package model

import "github.com/Yacobolo/leapview/internal/analytics/connectors"

const (
	KindPath   = connectors.KindPath
	KindObject = connectors.KindObject

	ScanTableFunction = connectors.ScanTableFunction
	ScanReplacement   = connectors.ScanReplacement

	AttachDatabase = connectors.AttachDatabase
	AttachDuckLake = connectors.AttachDuckLake

	ObjectRelationAttach = connectors.ObjectRelationAttach
)

type Format = connectors.Format

type ConnectionSpec = connectors.ConnectionSpec

func LookupFormat(name string) (Format, bool) {
	return connectors.LookupFormat(name)
}

func LookupConnection(kind string) (ConnectionSpec, bool) {
	return connectors.LookupConnection(kind)
}

func FormatNames() []string {
	return connectors.FormatNames()
}

func ConnectionKinds() []string {
	return connectors.ConnectionKinds()
}

func InferFormat(path string) (string, bool) {
	return connectors.InferFormat(path)
}

func IsLocalPath(path string) bool {
	return connectors.IsLocalPath(path)
}

func JoinScope(scope, path string) string {
	return connectors.JoinScope(scope, path)
}

func WithinScope(scope, path string) bool {
	return connectors.WithinScope(scope, path)
}

func StorageExtension(path string) (string, bool) {
	return connectors.StorageExtension(path)
}
