package proto

import (
	"encoding/json"

	"github.com/ubuntu/authd/api/types"
)

func (l *UILayout) ToLayout() (types.Layout, error) {
	data, err := json.Marshal(l)
	if err != nil {
		return types.Layout{}, err
	}

	return types.LayoutFromJSON(data)
}
