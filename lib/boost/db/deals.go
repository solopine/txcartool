package db

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/google/uuid"
	"github.com/solopine/txcartool/lib/boost/db/fielddef"
	"strings"
)

type FilterOptions struct {
	Checkpoint   *string
	IsOffline    *bool
	TransferType *string
	IsVerified   *bool
}

func withSearchQuery(fields []string, query string, searchLabel bool) (string, []interface{}) {
	query = strings.Trim(query, " \t\n")

	whereArgs := []interface{}{}
	where := "("
	for _, searchField := range fields {
		where += searchField + " = ? OR "
		whereArgs = append(whereArgs, query)
	}

	if searchLabel {
		// The label field is prefixed by the ' character
		// Note: In sqlite the concat operator is ||
		// Note: To escape a ' character it is prefixed by another '.
		// So when you put a ' in quotes, you have to write ''''
		where += "Label = ('''' || ?) OR "
		whereArgs = append(whereArgs, query)
	}

	where += " instr(Error, ?) > 0"
	whereArgs = append(whereArgs, query)
	where += ")"

	return where, whereArgs
}

func scan(fields []string, def map[string]fielddef.FieldDefinition, row Scannable) error {
	// For each field
	dest := []interface{}{}
	for _, name := range fields {
		// Get a pointer to the field that will receive the scanned value
		fieldDef := def[name]
		dest = append(dest, fieldDef.FieldPtr())
	}

	// Scan the row into each pointer
	err := row.Scan(dest...)
	if err != nil {
		return fmt.Errorf("scanning deal row: %w", err)
	}

	// For each field
	for name, fieldDef := range def {
		// Unmarshall the scanned value into deal object
		err := fieldDef.Unmarshall()
		if err != nil {
			return fmt.Errorf("unmarshalling db field %s: %s", name, err)
		}
	}
	return nil
}

func insert(ctx context.Context, table string, fields []string, fieldsStr string, def map[string]fielddef.FieldDefinition, db *sql.DB) error {
	// For each field
	values := []interface{}{}
	placeholders := make([]string, 0, len(values))
	for _, name := range fields {
		// Add a placeholder "?"
		fieldDef := def[name]
		placeholders = append(placeholders, "?")

		// Marshall the field into a value that can be stored in the database
		v, err := fieldDef.Marshall()
		if err != nil {
			return err
		}
		values = append(values, v)
	}

	// Execute the INSERT
	qry := "INSERT INTO " + table + " (" + fieldsStr + ") "
	qry += "VALUES (" + strings.Join(placeholders, ",") + ")"
	_, err := db.ExecContext(ctx, qry, values...)
	return err
}

func update(ctx context.Context, table string, fields []string, def map[string]fielddef.FieldDefinition, db *sql.DB, dealUuid uuid.UUID) error {
	// For each field
	values := []interface{}{}
	setNames := make([]string, 0, len(values))
	for _, name := range fields {
		// Skip the ID field
		if name == "ID" {
			continue
		}

		// Add "fieldName = ?"
		fieldDef := def[name]
		setNames = append(setNames, name+" = ?")

		// Marshall the field into a value that can be stored in the database
		v, err := fieldDef.Marshall()
		if err != nil {
			return err
		}
		values = append(values, v)
	}

	// Execute the UPDATE
	qry := "UPDATE " + table + " "
	qry += "SET " + strings.Join(setNames, ", ")

	qry += "WHERE ID = ?"
	values = append(values, dealUuid)

	_, err := db.ExecContext(ctx, qry, values...)
	return err
}
