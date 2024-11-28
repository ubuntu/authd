// Package types defines types shared by authd and the brokers.
package types

import "encoding/json"

// AuthMode is the type of authentication mode.
type AuthMode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// AuthModeFromJSON creates an AuthMode from JSON.
func AuthModeFromJSON(data []byte) (AuthMode, error) {
	var m AuthMode
	err := json.Unmarshal(data, &m)
	return m, err
}

// ToMap converts an AuthMode to a map of strings.
func (m AuthMode) ToMap() (map[string]string, error) {
	// Convert to JSON
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	// Convert to map
	var mp map[string]string
	err = json.Unmarshal(data, &mp)
	if err != nil {
		return nil, err
	}

	return mp, nil
}
