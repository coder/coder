package cliui

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

type OutputFormat interface {
	ID() string
	AttachFlags(cmd *cobra.Command)
	Format(ctx context.Context, data any) (string, error)
}

type OutputFormatter struct {
	formats  []OutputFormat
	formatID string
}

// NewOutputFormatter creates a new OutputFormatter with the given formats. The
// first format is the default format. At least two formats must be provided.
func NewOutputFormatter(formats ...OutputFormat) *OutputFormatter {
	if len(formats) < 2 {
		panic("at least two output formats must be provided")
	}

	formatIDs := make(map[string]struct{}, len(formats))
	for _, format := range formats {
		if format.ID() == "" {
			panic("output format ID must not be empty")
		}
		if _, ok := formatIDs[format.ID()]; ok {
			panic("duplicate format ID: " + format.ID())
		}
		formatIDs[format.ID()] = struct{}{}
	}

	return &OutputFormatter{
		formats:  formats,
		formatID: formats[0].ID(),
	}
}

// AttachFlags attaches the --output flag to the given command, and any
// additional flags required by the output formatters.
func (f *OutputFormatter) AttachFlags(cmd *cobra.Command) {
	for _, format := range f.formats {
		format.AttachFlags(cmd)
	}

	formatNames := make([]string, 0, len(f.formats))
	for _, format := range f.formats {
		formatNames = append(formatNames, format.ID())
	}

	cmd.Flags().StringVarP(&f.formatID, "output", "o", f.formats[0].ID(), "Output format. Available formats: "+strings.Join(formatNames, ", "))
}

// Format formats the given data using the format specified by the --output
// flag. If the flag is not set, the default format is used.
func (f *OutputFormatter) Format(ctx context.Context, data any) (string, error) {
	for _, format := range f.formats {
		if format.ID() == f.formatID {
			return format.Format(ctx, data)
		}
	}

	return "", xerrors.Errorf("unknown output format %q", f.formatID)
}

type tableFormat struct {
	defaultColumns []string
	allColumns     []string
	sort           string

	columns []string
}

var _ OutputFormat = &tableFormat{}

// TableFormat creates a table formatter for the given output type. The output
// type should be specified as an empty slice of the desired type.
//
// E.g.: TableFormat([]MyType{}, []string{"foo", "bar"})
//
// defaultColumns is optional and specifies the default columns to display. If
// not specified, all columns are displayed by default.
func TableFormat(out any, defaultColumns []string) OutputFormat {
	v := reflect.Indirect(reflect.ValueOf(out))
	if v.Kind() != reflect.Slice {
		panic("DisplayTable called with a non-slice type")
	}

	// Get the list of table column headers.
	headers, defaultSort, err := typeToTableHeaders(v.Type().Elem())
	if err != nil {
		panic("parse table headers: " + err.Error())
	}

	tf := &tableFormat{
		defaultColumns: headers,
		allColumns:     headers,
		sort:           defaultSort,
	}
	if len(defaultColumns) > 0 {
		tf.defaultColumns = defaultColumns
	}

	return tf
}

// ID implements OutputFormat.
func (*tableFormat) ID() string {
	return "table"
}

// AttachFlags implements OutputFormat.
func (f *tableFormat) AttachFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVarP(&f.columns, "column", "c", f.defaultColumns, "Columns to display in table output. Available columns: "+strings.Join(f.allColumns, ", "))
}

// Format implements OutputFormat.
func (f *tableFormat) Format(_ context.Context, data any) (string, error) {
	return DisplayTable(data, f.sort, f.columns)
}

type jsonFormat struct{}

var _ OutputFormat = jsonFormat{}

// JSONFormat creates a JSON formatter.
func JSONFormat() OutputFormat {
	return jsonFormat{}
}

// ID implements OutputFormat.
func (jsonFormat) ID() string {
	return "json"
}

// AttachFlags implements OutputFormat.
func (jsonFormat) AttachFlags(_ *cobra.Command) {}

// Format implements OutputFormat.
func (jsonFormat) Format(_ context.Context, data any) (string, error) {
	outBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", xerrors.Errorf("marshal output to JSON: %w", err)
	}

	return string(outBytes), nil
}
