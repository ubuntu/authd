package types

import "encoding/json"

type LayoutType string

type Layout struct {
	Type          LayoutType `json:"type"`
	Label         string     `json:"label"`
	Button        string     `json:"button"`
	Wait          string     `json:"wait"`
	Entry         string     `json:"entry"`
	Content       string     `json:"content"`
	Code          string     `json:"code"`
	RendersQrcode bool       `json:"renders_qrcode"`
}

func LayoutFromJSON(data []byte) (Layout, error) {
	var m Layout
	err := json.Unmarshal(data, &m)
	return m, err
}

func (m *Layout) MarshalJSON() ([]byte, error) {
	return json.Marshal(m)
}

func (m *Layout) ToMap() (map[string]string, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	var mp map[string]string
	err = json.Unmarshal(data, &mp)
	if err != nil {
		return nil, err
	}

	return mp, nil
}
