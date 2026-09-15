package focus

import (
	"io"

	"github.com/parquet-go/parquet-go"
)

// WriteParquet writes rows as a FOCUS Parquet document to w, using gzip
// compression: measured smallest among the codecs parquet-go supports for
// this schema (Phase One Draft, Parquet library selection). Row's `optional`
// pointer fields become nullable Parquet columns for free, matching FOCUS's
// nullable columns (ContractedUnitPrice, SubAccountId, and so on) exactly.
// Row values are written unescaped: formula-injection escaping is a
// CSV/spreadsheet-import concern only (see csv.go).
func WriteParquet(w io.Writer, rows []Row) error {
	return parquet.Write[Row](w, rows, parquet.Compression(&parquet.Gzip))
}
