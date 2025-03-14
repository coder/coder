package codersdk
import (
	"fmt"
	"errors"
	"bytes"
	"database/sql"
	"encoding/json"
	"time"
)
var nullBytes = []byte("null")
// NullTime represents a nullable time.Time.
// @typescript-ignore NullTime
type NullTime struct {
	sql.NullTime
}
// NewNullTime returns a new NullTime with the given time.Time.
func NewNullTime(t time.Time, valid bool) NullTime {
	return NullTime{
		NullTime: sql.NullTime{
			Time:  t,
			Valid: valid,
		},
	}
}
// MarshalJSON implements json.Marshaler.
func (t NullTime) MarshalJSON() ([]byte, error) {
	if !t.Valid {
		return []byte("null"), nil
	}
	b, err := t.Time.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("codersdk.NullTime: json encode failed: %w", err)
	}
	return b, nil
}
// UnmarshalJSON implements json.Unmarshaler.
func (t *NullTime) UnmarshalJSON(data []byte) error {
	t.Valid = false
	if bytes.Equal(data, nullBytes) {
		return nil
	}
	err := json.Unmarshal(data, &t.Time)
	if err != nil {
		return fmt.Errorf("codersdk.NullTime: json decode failed: %w", err)
	}
	t.Valid = true
	return nil
}
// IsZero return true if the time is null or zero.
func (t NullTime) IsZero() bool {
	return !t.Valid || t.Time.IsZero()
}
