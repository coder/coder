package cliui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

type OutputFormat interface {
	ID() string
	AttachOptions(opts *serpent.OptionSet)
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

// AttachOptions attaches the --output flag to the given command, and any
// additional flags required by the output formatters.
func (f *OutputFormatter) AttachOptions(opts *serpent.OptionSet) {
	for _, format := range f.formats {
		format.AttachOptions(opts)
	}

	formatNames := make([]string, 0, len(f.formats))
	for _, format := range f.formats {
		formatNames = append(formatNames, format.ID())
	}

	*opts = append(*opts,
		serpent.Option{
			Flag:          "output",
			FlagShorthand: "o",
			Default:       f.formats[0].ID(),
			Value:         serpent.EnumOf(&f.formatID, formatNames...),
			Description:   "Output format.",
		},
	)
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

// WriteLine formats data with Format and writes the result to w as a single
// line, appending a newline. Empty output is written as a bare newline.
func (f *OutputFormatter) WriteLine(ctx context.Context, w io.Writer, data any) error {
	out, err := f.Format(ctx, data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, out)
	return err
}

// FormatID will return the ID of the format selected by `--output`.
// If no flag is present, it returns the 'default' formatter.
func (f *OutputFormatter) FormatID() string {
	return f.formatID
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
//
// If the data is empty, an empty string is returned. Callers should check for
// this and provide an appropriate message to the user.
func TableFormat(out any, defaultColumns []string) OutputFormat {
	v := reflect.Indirect(reflect.ValueOf(out))
	if v.Kind() != reflect.Slice {
		panic("DisplayTable called with a non-slice type")
	}

	// Get the list of table column headers.
	headers, defaultSort, err := typeToTableHeaders(v.Type().Elem(), true)
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

// AttachOptions implements OutputFormat.
func (f *tableFormat) AttachOptions(opts *serpent.OptionSet) {
	*opts = append(*opts,
		serpent.Option{
			Flag:          "column",
			FlagShorthand: "c",
			Default:       strings.Join(f.defaultColumns, ","),
			Value:         serpent.EnumArrayOf(&f.columns, f.allColumns...),
			Description:   "Columns to display in table output.",
		},
	)
}

// Format implements OutputFormat.
func (f *tableFormat) Format(_ context.Context, data any) (string, error) {
	headers := make(table.Row, len(f.allColumns))
	for i, header := range f.allColumns {
		headers[i] = header
	}
	return renderTable(data, f.sort, headers, f.columns)
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

// AttachOptions implements OutputFormat.
func (jsonFormat) AttachOptions(_ *serpent.OptionSet) {}

// Format implements OutputFormat.
func (jsonFormat) Format(_ context.Context, data any) (string, error) {
	outBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", xerrors.Errorf("marshal output to JSON: %w", err)
	}

	return string(outBytes), nil
}

type textFormat struct{}

var _ OutputFormat = textFormat{}

// TextFormat is a formatter that just outputs unstructured text.
// It uses fmt.Sprintf under the hood.
func TextFormat() OutputFormat {
	return textFormat{}
}

func (textFormat) ID() string {
	return "text"
}

func (textFormat) AttachOptions(_ *serpent.OptionSet) {}

func (textFormat) Format(_ context.Context, data any) (string, error) {
	return fmt.Sprintf("%s", data), nil
}

// TextFormatFunc returns a text format that renders data of type T with fn.
// Formatting fails if the data passed to the formatter is not a T.
func TextFormatFunc[T any](fn func(T) (string, error)) OutputFormat {
	return ChangeFormatterData(TextFormat(), func(data any) (any, error) {
		typed, ok := data.(T)
		if !ok {
			return nil, xerrors.Errorf("expected %s, got %T", reflect.TypeFor[T](), data)
		}
		return fn(typed)
	})
}

// DataChangeFormat allows manipulating the data passed to an output format.
// This is because sometimes the data needs to be manipulated before it can be
// passed to the output format.
// For example, you may want to pass something different to the text formatter
// than what you pass to the json formatter.
type DataChangeFormat struct {
	format OutputFormat
	change func(data any) (any, error)
}

// ChangeFormatterData allows manipulating the data passed to an output
// format.
func ChangeFormatterData(format OutputFormat, change func(data any) (any, error)) *DataChangeFormat {
	return &DataChangeFormat{format: format, change: change}
}

func (d *DataChangeFormat) ID() string {
	return d.format.ID()
}

func (d *DataChangeFormat) AttachOptions(opts *serpent.OptionSet) {
	d.format.AttachOptions(opts)
}

func (d *DataChangeFormat) Format(ctx context.Context, data any) (string, error) {
	newData, err := d.change(data)
	if err != nil {
		return "", err
	}
	return d.format.Format(ctx, newData)
}
