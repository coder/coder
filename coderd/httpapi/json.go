package httpapi

import (
	"encoding/json"
	"time"

	"golang.org/x/xerrors"
)

// Duration wraps time.Duration and provides better JSON marshaling and
// unmarshalling. The default time.Duration marshals as an integer and only
// accepts integers when unmarshalling, which is not very user friendly as users
// cannot write durations like "1h30m".
//
// This type marshals as a string like "1h30m", and unmarshals from either a
// string or an integer.
type Duration time.Duration

var _ json.Marshaler = Duration(0)
var _ json.Unmarshaler = (*Duration)(nil)

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	err := json.Unmarshal(b, &v)
	if err != nil {
		return xerrors.Errorf("unmarshal JSON value: %w", err)
	}

	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value))
		return nil
	case string:
		tmp, err := time.ParseDuration(value)
		if err != nil {
			return xerrors.Errorf("parse duration %q: %w", value, err)
		}

		*d = Duration(tmp)
		return nil
	}

	return xerrors.New("invalid duration")
}
