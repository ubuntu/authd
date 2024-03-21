package gdm_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/pam/internal/gdm"
)

func reformatJSON(t *testing.T, input []byte) []byte {
	t.Helper()

	// We can't safely compare for JSON values when generated via protobuf
	// so we initially pass it to native implementation to make it rebuild
	// the JSON data so that the output is more reliable.
	// See: https://protobuf.dev/reference/go/faq/#unstable-json
	var js json.RawMessage
	err := json.Unmarshal(input, &js)
	require.NoError(t, err)
	out, err := json.Marshal(js)
	require.NoError(t, err)
	return out
}

func reformatJSONIndented(t *testing.T, input []byte) []byte {
	t.Helper()

	var indented bytes.Buffer
	err := json.Indent(&indented, input, "", "  ")
	require.NoError(t, err)
	return indented.Bytes()
}

func requireEqualData(t *testing.T, want *gdm.Data, actual *gdm.Data) {
	t.Helper()

	wantJSON, err := want.JSON()
	require.NoError(t, err)
	actualJSON, err := actual.JSON()
	require.NoError(t, err)

	require.Equal(t, string(reformatJSONIndented(t, wantJSON)),
		string(reformatJSONIndented(t, actualJSON)))
}

func TestGdmStructsMarshal(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		gdmData *gdm.Data

		wantJSON   string
		wantErrMsg string
	}{
		"Hello packet": {
			gdmData: &gdm.Data{Type: gdm.DataType_hello},

			wantJSON: `{"type":"hello"}`,
		},
		"Hello packet with data": {
			gdmData: &gdm.Data{Type: gdm.DataType_hello, Hello: &gdm.HelloData{Version: 55}},

			wantJSON: `{"type":"hello","hello":{"version":55}}`,
		},
		"Event packet": {
			gdmData: &gdm.Data{
				Type: gdm.DataType_event,
				Event: &gdm.EventData{
					Type: gdm.EventType_brokerSelected,
					Data: &gdm.EventData_BrokerSelected{},
				},
			},

			wantJSON: `{"type":"event","event":{"type":"brokerSelected","brokerSelected":{}}}`,
		},
		"Event ack packet": {
			gdmData: &gdm.Data{Type: gdm.DataType_eventAck},

			wantJSON: `{"type":"eventAck"}`,
		},
		"Request packet": {
			gdmData: &gdm.Data{
				Type: gdm.DataType_request,
				Request: &gdm.RequestData{
					Type: gdm.RequestType_uiLayoutCapabilities,
					Data: &gdm.RequestData_UiLayoutCapabilities{},
				},
			},

			wantJSON: `{"type":"request","request":{"type":"uiLayoutCapabilities","uiLayoutCapabilities":{}}}`,
		},
		"Request packet with missing data": {
			gdmData: &gdm.Data{
				Type: gdm.DataType_request,
				Request: &gdm.RequestData{
					Type: gdm.RequestType_updateBrokersList,
				},
			},

			wantJSON: `{"type":"request","request":{"type":"updateBrokersList"}}`,
		},
		"Response packet": {
			gdmData: &gdm.Data{
				Type: gdm.DataType_response,
				Response: &gdm.ResponseData{
					Type: gdm.RequestType_uiLayoutCapabilities,
					Data: &gdm.ResponseData_UiLayoutCapabilities{},
				},
			},

			wantJSON: `{"type":"response","response":{"type":"uiLayoutCapabilities","uiLayoutCapabilities":{}}}`,
		},
		"Response packet with ack data": {
			gdmData: &gdm.Data{
				Type: gdm.DataType_response,
				Response: &gdm.ResponseData{
					Type: gdm.RequestType_changeStage,
					Data: &gdm.ResponseData_Ack{},
				},
			},

			wantJSON: `{"type":"response","response":{"type":"changeStage","ack":{}}}`,
		},
		"Poll packet": {
			gdmData: &gdm.Data{Type: gdm.DataType_poll},

			wantJSON: `{"type":"poll"}`,
		},
		"PollResponse packet": {
			gdmData: &gdm.Data{
				Type: gdm.DataType_pollResponse,
				PollResponse: []*gdm.EventData{
					{
						Type: gdm.EventType_brokerSelected,
						Data: &gdm.EventData_BrokerSelected{
							BrokerSelected: &gdm.Events_BrokerSelected{BrokerId: "a broker"},
						},
					},
				},
			},

			wantJSON: `{"type":"pollResponse","pollResponse":` +
				`[{"type":"brokerSelected","brokerSelected":{"brokerId":"a broker"}}]}`,
		},
		"PollResponse packet with multiple results": {
			gdmData: &gdm.Data{
				Type: gdm.DataType_pollResponse,
				PollResponse: []*gdm.EventData{
					{
						Type: gdm.EventType_brokerSelected,
						Data: &gdm.EventData_BrokerSelected{
							BrokerSelected: &gdm.Events_BrokerSelected{BrokerId: "a broker"},
						},
					},
					{
						Type: gdm.EventType_authModeSelected,
						Data: &gdm.EventData_AuthModeSelected{
							AuthModeSelected: &gdm.Events_AuthModeSelected{AuthModeId: "auth mode"},
						},
					},
				},
			},

			wantJSON: `{"type":"pollResponse","pollResponse":` +
				`[{"type":"brokerSelected","brokerSelected":{"brokerId":"a broker"}},` +
				`{"type":"authModeSelected","authModeSelected":{"authModeId":"auth mode"}}]}`,
		},
		"PollResponse packet with nil data": {
			gdmData: &gdm.Data{
				Type:         gdm.DataType_pollResponse,
				PollResponse: nil,
			},

			wantJSON: `{"type":"pollResponse"}`,
		},
		"PollResponse packet with empty data": {
			gdmData: &gdm.Data{
				Type:         gdm.DataType_pollResponse,
				PollResponse: []*gdm.EventData{},
			},

			wantJSON: `{"type":"pollResponse"}`,
		},

		// Error cases
		"Error empty packet": {
			gdmData: &gdm.Data{},

			wantErrMsg: "unexpected type unknownType",
		},
		"Error if packet has invalid type": {
			gdmData: &gdm.Data{Type: gdm.DataType(-1)},

			wantErrMsg: "unhandled type -1",
		},
		"Error hello packet with unexpected data": {
			gdmData: &gdm.Data{Type: gdm.DataType_hello, Request: &gdm.RequestData{}},

			wantErrMsg: "field Request should not be defined",
		},
		"Error event packet with unknown type": {
			gdmData: &gdm.Data{
				Type:  gdm.DataType_event,
				Event: &gdm.EventData{Type: gdm.EventType_unknownEvent},
			},

			wantErrMsg: "missing event type",
		},
		"Error event packet with invalid type": {
			gdmData: &gdm.Data{Type: gdm.DataType_event, Event: &gdm.EventData{Type: gdm.EventType(-1)}},

			wantErrMsg: "unexpected event type",
		},
		"Error event packet with missing data": {
			gdmData: &gdm.Data{Type: gdm.DataType_event, Event: nil},

			wantErrMsg: "missing event data",
		},
		"Error event packet with empty data": {
			gdmData: &gdm.Data{Type: gdm.DataType_event, Event: &gdm.EventData{}},

			wantErrMsg: "missing event type",
		},
		"Error event packet with missing type": {
			gdmData: &gdm.Data{Type: gdm.DataType_event, Event: &gdm.EventData{Data: &gdm.EventData_AuthModeSelected{}}},

			wantErrMsg: "missing event type",
		},
		"Error event packet with unexpected data": {
			gdmData: &gdm.Data{
				Type:  gdm.DataType_event,
				Event: &gdm.EventData{Type: gdm.EventType_authEvent, Data: &gdm.EventData_AuthModeSelected{}},
				Hello: &gdm.HelloData{},
			},

			wantErrMsg: "field Hello should not be defined",
		},
		"Error event ack packet with unexpected data": {
			gdmData: &gdm.Data{Type: gdm.DataType_eventAck, Event: &gdm.EventData{}},

			wantErrMsg: "field Event should not be defined",
		},
		"Error request packet with unknown type": {
			gdmData: &gdm.Data{Type: gdm.DataType_request, Request: &gdm.RequestData{Data: &gdm.RequestData_ChangeStage{}}},

			wantErrMsg: "missing request type",
		},
		"Error request packet with invalid type": {
			gdmData: &gdm.Data{Type: gdm.DataType_request, Request: &gdm.RequestData{Type: gdm.RequestType(-1)}},

			wantErrMsg: "unexpected request type",
		},
		"Error request packet with missing data": {
			gdmData: &gdm.Data{Type: gdm.DataType_request, Request: nil},

			wantErrMsg: "missing request data",
		},
		"Error request packet with empty data": {
			gdmData:    &gdm.Data{Type: gdm.DataType_request, Request: &gdm.RequestData{}},
			wantErrMsg: "missing request type",
		},
		"Error request packet with unexpected data": {
			gdmData: &gdm.Data{
				Type: gdm.DataType_request,
				Request: &gdm.RequestData{
					Type: gdm.RequestType_changeStage,
					Data: &gdm.RequestData_ChangeStage{},
				},
				Event: &gdm.EventData{},
			},

			wantErrMsg: "field Event should not be defined",
		},
		"Error response packet with missing data": {
			gdmData: &gdm.Data{Type: gdm.DataType_response},

			wantErrMsg: "missing response data",
		},
		"Error response packet with missing type": {
			gdmData: &gdm.Data{
				Type:     gdm.DataType_response,
				Response: &gdm.ResponseData{Data: &gdm.ResponseData_Ack{}},
			},

			wantErrMsg: "missing response type",
		},
		"Error response packet with invalid type": {
			gdmData: &gdm.Data{
				Type:     gdm.DataType_response,
				Response: &gdm.ResponseData{Type: gdm.RequestType(-1), Data: &gdm.ResponseData_Ack{}},
			},

			wantErrMsg: "unexpected request type -1",
		},
		"Error response packet with unexpected data": {
			gdmData: &gdm.Data{
				Type:     gdm.DataType_response,
				Response: &gdm.ResponseData{Type: gdm.RequestType_changeStage, Data: &gdm.ResponseData_Ack{}},
				Event:    &gdm.EventData{},
			},

			wantErrMsg: "field Event should not be defined",
		},
		"Error poll packet with unexpected data": {
			gdmData: &gdm.Data{Type: gdm.DataType_poll, Request: &gdm.RequestData{}},

			wantErrMsg: "field Request should not be defined",
		},
		"Error pollResponse packet with missing event type": {
			gdmData: &gdm.Data{
				Type: gdm.DataType_pollResponse,
				PollResponse: []*gdm.EventData{
					{
						Type: gdm.EventType_brokerSelected,
						Data: &gdm.EventData_BrokerSelected{
							BrokerSelected: &gdm.Events_BrokerSelected{BrokerId: "a broker"},
						},
					},
					{
						Data: &gdm.EventData_AuthModeSelected{
							AuthModeSelected: &gdm.Events_AuthModeSelected{AuthModeId: "auth mode"},
						},
					},
				},
			},

			wantErrMsg: "poll response data member 1 invalid: missing event type",
		},
		"Error pollResponse packet with event with missing type": {
			gdmData: &gdm.Data{
				Type: gdm.DataType_pollResponse,
				PollResponse: []*gdm.EventData{
					{},
					{
						Type: gdm.EventType_authModeSelected,
						Data: &gdm.EventData_AuthModeSelected{
							AuthModeSelected: &gdm.Events_AuthModeSelected{AuthModeId: "auth mode"},
						},
					},
				},
			},

			wantErrMsg: "poll response data member 0 invalid: missing event type",
		},
		"Error pollResponse packet with unexpected data": {
			gdmData: &gdm.Data{
				Type:         gdm.DataType_pollResponse,
				PollResponse: []*gdm.EventData{},
				Event:        &gdm.EventData{},
			},

			wantErrMsg: "field Event should not be defined",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			bytes, err := tc.gdmData.JSON()
			if tc.wantErrMsg != "" {
				require.ErrorContains(t, err, tc.wantErrMsg)
				return
			}
			require.NoError(t, err)

			formattedJSON := string(reformatJSON(t, bytes))
			require.Equal(t, tc.wantJSON, formattedJSON)

			// Now try to reconvert things back again
			gdmData, err := gdm.NewDataFromJSON(bytes)
			require.NoError(t, err)
			requireEqualData(t, tc.gdmData, gdmData)
		})
	}
}

func TestGdmStructsUnMarshal(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		JSON string

		wantData   *gdm.Data
		wantErrMsg string
	}{
		"hello packet": {
			JSON: `{"type":"hello"}`,

			wantData: &gdm.Data{Type: gdm.DataType_hello},
		},
		"Hello packet with data": {
			JSON: `{"type":"hello","hello":{"version":55}}`,

			wantData: &gdm.Data{Type: gdm.DataType_hello, Hello: &gdm.HelloData{Version: 55}},
		},
		"Event packet": {
			JSON: `{"type":"event","event":{"type":"brokerSelected","brokerSelected":{}}}`,

			wantData: &gdm.Data{
				Type: gdm.DataType_event,
				Event: &gdm.EventData{
					Type: gdm.EventType_brokerSelected,
					Data: &gdm.EventData_BrokerSelected{},
				},
			},
		},
		"Event ack packet": {
			JSON: `{"type":"eventAck"}`,

			wantData: &gdm.Data{Type: gdm.DataType_eventAck},
		},
		"Request packet": {
			JSON: `{"type":"request","request":{"type":"uiLayoutCapabilities","uiLayoutCapabilities":{}}}`,

			wantData: &gdm.Data{
				Type: gdm.DataType_request,
				Request: &gdm.RequestData{
					Type: gdm.RequestType_uiLayoutCapabilities,
					Data: &gdm.RequestData_UiLayoutCapabilities{},
				},
			},
		},
		"Request packet with missing data": {
			JSON: `{"type":"request","request":{"type":"updateBrokersList"}}`,

			wantData: &gdm.Data{
				Type: gdm.DataType_request,
				Request: &gdm.RequestData{
					Type: gdm.RequestType_updateBrokersList,
				},
			},
		},
		"Response packet": {
			JSON: `{"type":"response","response":{"type":"uiLayoutCapabilities","uiLayoutCapabilities":{}}}`,

			wantData: &gdm.Data{
				Type: gdm.DataType_response,
				Response: &gdm.ResponseData{
					Type: gdm.RequestType_uiLayoutCapabilities,
					Data: &gdm.ResponseData_UiLayoutCapabilities{},
				},
			},
		},
		"Response packet with ack data": {
			JSON: `{"type":"response","response":{"type":"changeStage","ack":{}}}`,

			wantData: &gdm.Data{
				Type: gdm.DataType_response,
				Response: &gdm.ResponseData{
					Type: gdm.RequestType_changeStage,
					Data: &gdm.ResponseData_Ack{},
				},
			},
		},
		"Poll packet": {
			JSON: `{"type":"poll"}`,

			wantData: &gdm.Data{Type: gdm.DataType_poll},
		},
		"PollResponse packet": {
			JSON: `{"type":"pollResponse","pollResponse":` +
				`[{"type":"brokerSelected","brokerSelected":{"brokerId":"a broker"}}]}`,

			wantData: &gdm.Data{
				Type: gdm.DataType_pollResponse,
				PollResponse: []*gdm.EventData{
					{
						Type: gdm.EventType_brokerSelected,
						Data: &gdm.EventData_BrokerSelected{
							BrokerSelected: &gdm.Events_BrokerSelected{BrokerId: "a broker"},
						},
					},
				},
			},
		},
		"PollResponse packet with missing data": {
			JSON: `{"type":"pollResponse"}`,

			wantData: &gdm.Data{
				Type:         gdm.DataType_pollResponse,
				PollResponse: nil,
			},
		},

		// Error cases
		"Error empty packet ": {
			wantErrMsg: "syntax error",
		},
		"Error empty packet object": {
			JSON: `{}`,

			wantErrMsg: "unexpected type unknownType",
		},
		"Error packet with invalid type": {
			JSON: `{"type":"invalidType"}`,

			wantErrMsg: "invalid value for enum type",
		},
		"Error packet with invalid value type": {
			JSON: `{"type":[]}`,

			wantErrMsg: "invalid value for enum type",
		},
		"Error hello packet with unexpected data": {
			JSON: `{"type":"hello","request":{}}`,

			wantErrMsg: "field Request should not be defined",
		},
		"Error event packet with invalid data": {
			JSON: `{"type":"event","fooEvent":null}`,

			wantErrMsg: `unknown field "fooEvent"`,
		},
		"Error event packet with missing type": {
			JSON:       `{"type":"event","event":{}}`,
			wantErrMsg: "missing event type",
		},
		"Error event packet with unknown type": {
			JSON: `{"type":"event","event":{"type":"someType"}`,

			wantErrMsg: "invalid value for enum type",
		},
		"Error event packet with invalid value type": {
			JSON: `{"type":"event","event":{"brokerSelected":{},"type":{}}}`,

			wantErrMsg: "invalid value for enum type",
		},
		"Error event packet with missing data": {
			JSON: `{"type":"event","event":{"type":"brokerSelected"}}`,

			wantErrMsg: "missing event data",
		},
		"Error event packet with unexpected data": {
			JSON: `{"type":"event","event":{"type":"brokerSelected",` +
				`"brokerSelected":{}},"request":{}}`,

			wantErrMsg: "field Request should not be defined",
		},
		"Error event ack packet with unexpected member": {
			JSON: `{"type":"eventAck","event":{}}`,

			wantErrMsg: "field Event should not be defined",
		},
		"Error request packet with missing type": {
			JSON: `{"type":"request","request":{"uiLayoutCapabilities":{}}}`,

			wantErrMsg: "missing request type",
		},
		"Error request packet with unknown type": {
			JSON: `{"type":"request","request":{"type":true,"uiLayoutCapabilities":{}}}`,

			wantErrMsg: "invalid value for enum type",
		},
		"Error request packet with unknown value type": {
			JSON: `{"type":"request","request":{"type":"someUnknownRequest",` +
				`"uiLayoutCapabilities":{}}}`,

			wantErrMsg: "invalid value for enum type",
		},
		"Error request packet with unexpected data": {
			JSON: `{"type":"request","request":{"type": "uiLayoutCapabilities",` +
				`"uiLayoutCapabilities":{}}, "event":{}}`,

			wantErrMsg: "field Event should not be defined",
		},
		"Error response packet with missing data": {
			JSON: `{"type":"response"}`,

			wantErrMsg: "missing response data",
		},
		"Error response packet with unexpected data": {
			JSON: `{"type":"response","response":{"type":"changeStage","ack":{}}, "event":{}}`,

			wantErrMsg: "field Event should not be defined",
		},
		"Error poll packet with unexpected data": {
			JSON: `{"type":"poll", "response": {}}`,

			wantErrMsg: "field Response should not be defined",
		},
		"Error pollResponse packet with missing event type": {
			JSON: `{"type":"pollResponse","pollResponse":` +
				`[{"type":"brokerSelected","brokerSelected":{"brokerId":"a broker"}},` +
				`{"authModeSelected":{"authModeId":"auth mode"}}]}`,

			wantErrMsg: "poll response data member 1 invalid: missing event type",
		},
		"Error pollResponse packet with unsupported event type": {
			JSON: `{"type":"pollResponse","pollResponse":` +
				`[{"type":"brokerSelected","brokerSelected":{"brokerId":"a broker"}},` +
				`{"type":"invalidEvent"}]}`,

			wantErrMsg: "invalid value for enum type",
		},
		"Error pollResponse packet with unexpected data": {
			JSON: `{"type":"pollResponse","pollResponse":` +
				`[{"type":"brokerSelected","brokerSelected":{"brokerId":"a broker"}},` +
				`{"type":"authModeSelected","authModeSelected":{"authModeId":"auth mode"}}],` +
				`"response":{}}`,

			wantErrMsg: "field Response should not be defined",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gdmData, err := gdm.NewDataFromJSON([]byte(tc.JSON))
			if tc.wantErrMsg != "" {
				require.ErrorContains(t, err, tc.wantErrMsg)
				return
			}
			require.NoError(t, err)

			requireEqualData(t, tc.wantData, gdmData)

			// Convert back the data to JSON and check it's still matching.
			json, err := gdmData.JSON()
			require.NoError(t, err)
			require.Equal(t, tc.JSON, string(reformatJSON(t, json)))
		})
	}
}
