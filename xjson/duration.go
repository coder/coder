package xjson

import (
	"encoding/json"
	"strconv"
	"time"
)

// Duration is a time.Duration that marshals to millisecond precision.
// Most javascript applications expect durations to be in milliseconds.
// Although this would typically be a time.Duration it was changed to
// an int64 to avoid errors in the swaggo/swag tool we use to auto -generate
// documentation.
type Duration int64

// MarshalJSON marshals the duration to millisecond precision.
func (d Duration) MarshalJSON() ([]byte, error) {
	du := time.Duration(d)
	return json.Marshal(du.Milliseconds())
}

// UnmarshalJSON unmarshals a millisecond-precision integer to
// a time.Duration.
func (d *Duration) UnmarshalJSON(b []byte) error {
	i, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return err
	}

	*d = Duration(time.Duration(i) * time.Millisecond)
	return nil
}
