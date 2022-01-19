package srverr

import (
	"encoding/json"
)

// Upgrade transparently upgrades any error chain by adding information on how
// the error should be converted into an HTTP response. Since this adds it to
// the chain transparently, there is no indication from the error string that
// it is an upgraded error. You must use xerrors.As to check if an error chain
// contains an upgraded error.
// An error may be upgraded multiple times. The last call to Upgrade will
// always be used.
func Upgrade(err error, herr Error) error {
	return wrapped{
		err:  err,
		herr: herr,
	}
}

var _ VerboseError = wrapped{}

type wrapped struct {
	err  error
	herr Error
}

// Make sure the wrapped error still behaves as if it was a regular call to
// xerrors.Errorf.
func (w wrapped) Error() string { return w.err.Error() }
func (w wrapped) Unwrap() error { return w.err }

// Pass through srverr.Error interface functions from the underlying
// srverr.Error.
func (w wrapped) Status() int           { return w.herr.Status() }
func (w wrapped) PublicMessage() string { return w.herr.PublicMessage() }
func (w wrapped) Code() Code            { return w.herr.Code() }

// When a wrapped error is marshaled, we want to make sure it marshals the
// underlying srverr.Error, not the wrapped structure.
func (w wrapped) MarshalJSON() ([]byte, error) { return json.Marshal(w.herr) }

// If the underlying srverr.Error implements VerboseError, pass through.
func (w wrapped) SetVerbose(err error) {
	if v, ok := w.herr.(VerboseError); ok {
		v.SetVerbose(err)
	}
}
