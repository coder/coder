package dbtypes

import (
	"database/sql/driver"

	"golang.org/x/xerrors"
	"tailscale.com/types/key"
)

type NodePublic key.NodePublic

func (n NodePublic) String() string {
	return key.NodePublic(n).String()
}

func (n NodePublic) MarshalJSON() ([]byte, error) {
	j, err := key.NodePublic(n).MarshalText()
	// surround in quotes to make it a JSON string
	j = append([]byte{'"'}, j...)
	j = append(j, '"')
	return j, err
}

func (n NodePublic) Value() (driver.Value, error) {
	return key.NodePublic(n).MarshalText()
}

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

type DiscoPublic key.DiscoPublic

func (n DiscoPublic) String() string {
	return key.DiscoPublic(n).String()
}

func (n DiscoPublic) MarshalJSON() ([]byte, error) {
	j, err := key.DiscoPublic(n).MarshalText()
	// surround in quotes to make it a JSON string
	j = append([]byte{'"'}, j...)
	j = append(j, '"')
	return j, err
}

func (n DiscoPublic) Value() (driver.Value, error) {
	return key.DiscoPublic(n).MarshalText()
}

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
