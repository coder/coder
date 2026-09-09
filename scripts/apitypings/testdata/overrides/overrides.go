package overrides

import "encoding/json"

type Overrides struct {
	Field json.RawMessage
}

type ChatMCPApp struct {
	Result json.RawMessage `json:"result"`
}
