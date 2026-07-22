#!/bin/sh
set -eu

APIGEN=github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.5.3

go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
go run ./internal/tools/configgen

go run ./internal/tools/apigenstage -target leapview-v1
go run "$APIGEN" typespec-compile -manifest api/apigen.yaml -target leapview-v1
go run "$APIGEN" all -manifest api/apigen.yaml -target leapview-v1

go run ./internal/tools/apigenstage -target ui-signals
go run "$APIGEN" typespec-compile -manifest api/apigen.yaml -target ui-signals
go run "$APIGEN" all -manifest api/apigen.yaml -target ui-signals
go run ./internal/tools/uisignalspostprocess

go run "$APIGEN" typespec-compile -manifest api/apigen.yaml -target visualization-ir
go run "$APIGEN" all -manifest api/apigen.yaml -target visualization-ir
go run ./internal/tools/uisignalspostprocess -go-models internal/visualization/ir/models.gen.go -typescript=

go run ./cmd/leapview schema export --format json-schema --out schemas/json
