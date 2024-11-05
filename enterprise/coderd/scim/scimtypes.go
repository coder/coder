package scim

import "time"

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
