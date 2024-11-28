package types

import "encoding/json"

type AuthMode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func AuthModeFromJSON(data []byte) (AuthMode, error) {
	var m AuthMode
	err := json.Unmarshal(data, &m)
	return m, err
}
