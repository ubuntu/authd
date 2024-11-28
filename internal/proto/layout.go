package proto

import (
	"encoding/json"

	"github.com/ubuntu/authd/api/types"
)

// ToMap converts a Layout to a map of strings.
func (l *UILayout) ToMap() (map[string]string, error) {
	data, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}

	// Check if the JSON can be successfully unmarshalled into the Layout struct
	_, err = types.LayoutFromJSON(data)
	if err != nil {
		return nil, err
	}

	var m map[string]string
	err = json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	return m, nil
}
