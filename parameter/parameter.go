package parameter

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"
)

type Scheme string

const (
	SchemeText        = "text"
	SchemeEnvironment = "env"
	SchemeVariable    = "var"
)

// URI represents a parameter source and destination scheme.
type URI struct {
	Scheme Scheme
	Value  string
}

func (u URI) String() string {
	return fmt.Sprintf("%s://%s", u.Scheme, u.Value)
}

func Parse(rawURI string) (URI, error) {
	parts := strings.SplitN(rawURI, "://", 2)
	uri := URI{
		Value: parts[1],
	}
	switch parts[0] {
	case SchemeText:
		uri.Scheme = SchemeText
	case SchemeEnvironment:
		uri.Scheme = SchemeEnvironment
	case SchemeVariable:
		uri.Scheme = SchemeVariable
	default:
		return uri, xerrors.Errorf("unrecognized scheme: %s", parts[0])
	}
	return uri, nil
}
