package dbtypes

import (
	"database/sql/driver"

	"golang.org/x/xerrors"
	"tailscale.com/types/key"
)

// NodePublic is a wrapper around a key.NodePublic which represents the
// Wireguard public key for an agent..
type NodePublic key.NodePublic

func (n NodePublic) String() string {
	return key.NodePublic(n).String()
}

// This is necessary so NodePublic can be serialized in JSON loggers.
func (n NodePublic) MarshalJSON() ([]byte, error) {
	j, err := key.NodePublic(n).MarshalText()
	// surround in quotes to make it a JSON string
	j = append([]byte{'"'}, append(j, '"')...)
	return j, err
}

// Value is so NodePublic can be inserted into the database.
func (n NodePublic) Value() (driver.Value, error) {
	return key.NodePublic(n).MarshalText()
}

// Scan is so NodePublic can be read from the database.
func (n *NodePublic) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		return (*key.NodePublic)(n).UnmarshalText(v)
	case string:
		return (*key.NodePublic)(n).UnmarshalText([]byte(v))
	default:
		return xerrors.Errorf("unexpected type: %T", v)
	}
}

// NodePublic is a wrapper around a key.NodePublic which represents the
// Tailscale disco key for an agent.
type DiscoPublic key.DiscoPublic

func (n DiscoPublic) String() string {
	return key.DiscoPublic(n).String()
}

// This is necessary so DiscoPublic can be serialized in JSON loggers.
func (n DiscoPublic) MarshalJSON() ([]byte, error) {
	j, err := key.DiscoPublic(n).MarshalText()
	// surround in quotes to make it a JSON string
	j = append([]byte{'"'}, append(j, '"')...)
	return j, err
}

// Value is so DiscoPublic can be inserted into the database.
func (n DiscoPublic) Value() (driver.Value, error) {
	return key.DiscoPublic(n).MarshalText()
}

// Scan is so DiscoPublic can be read from the database.
func (n *DiscoPublic) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		return (*key.DiscoPublic)(n).UnmarshalText(v)
	case string:
		return (*key.DiscoPublic)(n).UnmarshalText([]byte(v))
	default:
		return xerrors.Errorf("unexpected type: %T", v)
	}
}
