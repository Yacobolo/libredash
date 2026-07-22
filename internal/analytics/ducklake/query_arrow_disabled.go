//go:build !duckdb_arrow

package ducklake

import (
	"context"
	"fmt"

	"github.com/Yacobolo/leapview/internal/analytics/arrowquery"
	semanticquery "github.com/Yacobolo/leapview/internal/analytics/query"
)

const nativeArrowEnabled = false

func (e *Environment) QueryArrow(context.Context, semanticquery.Plan, arrowquery.Sink) error {
	return fmt.Errorf("native Arrow execution requires the duckdb_arrow build tag")
}
