package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type Response struct {
	Message string  `json:"message"`
	Errors  []Error `json:"errors,omitempty"`
}

type Error struct {
	Resource string `json:"resource"`
	Field    string `json:"field"`
}

// Write outputs a standardized format to an HTTP response body.
func Write(w http.ResponseWriter, status int, response Response) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(true)
	err := enc.Encode(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, err = w.Write(buf.Bytes())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
