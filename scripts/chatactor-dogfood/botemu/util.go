package main

import (
	"encoding/json"
	"io"

	"github.com/lib/pq"
)

func decodeJSON(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}

func unmarshalJSON(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

// pqStringArray scans a Postgres text[] column.
type pqStringArray []string

func (a *pqStringArray) Scan(src any) error {
	return (*pq.StringArray)(a).Scan(src)
}
