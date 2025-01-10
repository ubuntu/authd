package brokers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils/golden"
)

// These are used to test the JSON unmarshaling of the User struct.
var (
	completeJSON = `
{
	"Name":"success",
	"UID":82162,
	"Gecos":"gecos for success",
	"Dir":"/home/success",
	"Shell":"/bin/sh/success",
	"Groups":[
		{"Name":"success","GID":82162},
		{"Name":"group-success","GID":81868}
	]
}`
	emptyFieldJSON = `
{
	"Name":"",
	"UID":82162,
	"Gecos":"gecos for success",
	"Dir":"/home/success",
	"Shell":"/bin/sh/success",
	"Groups":[
		{"Name":"success","GID":82162},
		{"Name":"group-success","GID":81868}
	]
}`
	missingFieldJSON = `
{
	"UID":82162,
	"Gecos":"gecos for success",
	"Dir":"/home/success",
	"Shell":"/bin/sh/success",
	"Groups":[
		{"Name":"success","GID":82162},
		{"Name":"group-success","GID":81868}
	]
}`
	additionalFieldJSON = `
{
	"Name":"success",
	"UID":82162,
	"Gecos":"gecos for success",
	"Dir":"/home/success",
	"AdditionalFieldNotInStruct":"what's this?",
	"Shell":"/bin/sh/success",
	"Groups":[
		{"Name":"success","GID":82162},
		{"Name":"group-success","GID":81868}
	]
}`
)

func TestUnmarshalUserInfo(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		jsonInput string

		wantErr bool
	}{
		"Successfully unmarshal complete user info":            {jsonInput: completeJSON},
		"Unmarshaling json with empty field keeps its value":   {jsonInput: emptyFieldJSON},
		"Unmarshaling json with missing field adds zero value": {jsonInput: missingFieldJSON},
		"Unmarshaling json with additional field ignores it":   {jsonInput: additionalFieldJSON},

		"Error when unmarshaling invalid json": {jsonInput: "invalid-json", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := unmarshalUserInfo([]byte(tc.jsonInput))
			if tc.wantErr {
				require.Error(t, err, "unmarshalUserInfo should return an error, but did not")
				return
			}
			require.NoError(t, err, "unmarshalUserInfo should not return an error, but did")

			gotJSON, err := json.Marshal(got)
			require.NoError(t, err, "Marshaling the result should not return an error, but did")

			golden.CheckOrUpdate(t, string(gotJSON))
		})
	}
}
