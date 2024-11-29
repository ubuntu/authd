package proto

import (
	"encoding/json"
	"fmt"

	"github.com/ubuntu/authd/api/types"
)

// ToMap converts a Layout to a map of strings.
func (l *UILayout) ToMap() (map[string]string, error) {
	data, err := json.Marshal(l)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Layout to JSON: %w", err)
	}

	// Check if the JSON can be successfully unmarshalled into the Layout struct
	_, err = types.LayoutFromJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to Layout: %w", err)
	}

	var m map[string]string
	err = json.Unmarshal(data, &m)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return m, nil
}

func (l *UILayout) MarshalJSON() ([]byte, error) {
	fmt.Fprintf(os.Stderr, "XXX: UILayout.MarshalJSON(%#v)\n", l)
	// The rendersQrcode field must be marshalled as a string.
	var rendersQrcodeStr string
	if l.RendersQrcode != nil {
		if *l.RendersQrcode {
			rendersQrcodeStr = "true"
		} else {
			rendersQrcodeStr = "false"
		}
	}

	type Alias UILayout
	return json.Marshal(&struct {
		RendersQrcode string `json:"renders_qrcode"`
		*Alias
	}{
		RendersQrcode: rendersQrcodeStr,
		Alias:         (*Alias)(l),
	})
}
