package scim

import (
	"encoding/json"
	"time"

	"github.com/imulab/go-scim/pkg/v2/spec"
)

type ServiceProviderConfig struct {
	Schemas        []string               `json:"schemas"`
	DocURI         string                 `json:"documentationUri"`
	Patch          Supported              `json:"patch"`
	Bulk           BulkSupported          `json:"bulk"`
	Filter         FilterSupported        `json:"filter"`
	ChangePassword Supported              `json:"changePassword"`
	Sort           Supported              `json:"sort"`
	ETag           Supported              `json:"etag"`
	AuthSchemes    []AuthenticationScheme `json:"authenticationSchemes"`
	Meta           ServiceProviderMeta    `json:"meta"`
}

type ServiceProviderMeta struct {
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	Location     string    `json:"location"`
	ResourceType string    `json:"resourceType"`
}

type Supported struct {
	Supported bool `json:"supported"`
}

type BulkSupported struct {
	Supported  bool `json:"supported"`
	MaxOp      int  `json:"maxOperations"`
	MaxPayload int  `json:"maxPayloadSize"`
}

type FilterSupported struct {
	Supported  bool `json:"supported"`
	MaxResults int  `json:"maxResults"`
}

type AuthenticationScheme struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SpecURI     string `json:"specUri"`
	DocURI      string `json:"documentationUri"`
}

// HTTPError wraps a *spec.Error for correct usage with
// 'handlerutil.WriteError'. This error type is cursed to be
// absolutely strange and specific to the SCIM library we use.
//
// The library expects *spec.Error to be returned on unwrap, and the
// internal error description to be returned by a json.Marshal of the
// top level error.
type HTTPError struct {
	scim     *spec.Error
	internal error
}

func NewHTTPError(status int, eType string, err error) *HTTPError {
	return &HTTPError{
		scim: &spec.Error{
			Status: status,
			Type:   eType,
		},
		internal: err,
	}
}

func (e HTTPError) Error() string {
	return e.internal.Error()
}

func (e HTTPError) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.internal)
}

func (e HTTPError) Unwrap() error {
	return e.scim
}
