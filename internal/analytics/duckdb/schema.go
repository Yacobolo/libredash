package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
)

func DiscoverSchemas(ctx context.Context, db *Database, model *semanticmodel.Model) error {
	if db == nil || db.SQLDB() == nil {
		return fmt.Errorf("schema discovery requires a DuckDB database")
	}
	if model == nil {
		return fmt.Errorf("schema discovery requires a semantic model")
	}
	var databaseName string
	if err := db.SQLDB().QueryRowContext(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		return err
	}
	rows, err := db.SQLDB().QueryContext(ctx, `
SELECT schema_name, table_name, column_name, column_index, data_type, is_nullable, column_default, comment
FROM duckdb_columns()
WHERE database_name = ? AND schema_name IN ('source', 'model')
ORDER BY schema_name, table_name, column_index`, databaseName)
	if err != nil {
		return err
	}
	defer rows.Close()

	sourceColumns := map[string][]semanticmodel.ColumnSchema{}
	tableColumns := map[string][]semanticmodel.ColumnSchema{}
	for rows.Next() {
		var schemaName, tableName, columnName, dataType string
		var ordinal int
		var nullable sql.NullBool
		var defaultValue, comment sql.NullString
		if err := rows.Scan(&schemaName, &tableName, &columnName, &ordinal, &dataType, &nullable, &defaultValue, &comment); err != nil {
			return err
		}
		var nullableValue *bool
		if nullable.Valid {
			value := nullable.Bool
			nullableValue = &value
		}
		column := semanticmodel.ColumnSchema{
			Name:         columnName,
			Ordinal:      ordinal,
			PhysicalType: dataType,
			Nullable:     nullableValue,
			Default:      defaultValue.String,
			Comment:      comment.String,
		}
		switch schemaName {
		case "source":
			sourceColumns[tableName] = append(sourceColumns[tableName], column)
		case "model":
			tableColumns[tableName] = append(tableColumns[tableName], column)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for name, source := range model.Sources {
		source.Schema = semanticmodel.TableSchema{Columns: sortedColumns(sourceColumns[name])}
		model.Sources[name] = source
	}
	for name, table := range model.Tables {
		columns := sortedColumns(tableColumns[name])
		for index := range columns {
			columns[index].PrimaryKey = columns[index].Name == table.PrimaryKey
		}
		table.Schema = semanticmodel.TableSchema{Columns: columns}
		model.Tables[name] = table
	}
	return model.ValidateDiscoveredSchemas()
}

func (db *Database) DiscoverSchemas(ctx context.Context, model *semanticmodel.Model) error {
	return DiscoverSchemas(ctx, db, model)
}

func sortedColumns(columns []semanticmodel.ColumnSchema) []semanticmodel.ColumnSchema {
	out := append([]semanticmodel.ColumnSchema{}, columns...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ordinal != out[j].Ordinal {
			return out[i].Ordinal < out[j].Ordinal
		}
		return out[i].Name < out[j].Name
	})
	return out
}
