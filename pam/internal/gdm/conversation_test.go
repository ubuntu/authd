package gdm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	authd "github.com/ubuntu/authd"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

func TestSendToGdm(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		value []byte

		wantReturn                   []byte
		wantError                    error
		wantConvHandlerNotToBeCalled bool
	}{
		"JSON null data can be sent and received": {
			value: []byte(`null`),
		},
		"JSON number can be sent and received": {
			value: []byte(`1.5`),
		},
		"Single char is sent and received as string": {
			value: []byte(`"m"`),
		},
		"JSON null is returned": {
			value:      []byte(`"give me 🚫"`),
			wantReturn: []byte("null"),
		},
		"Utf-8 data is sent and returned": {
			value:      []byte(`"give me 🍕"`),
			wantReturn: []byte(`"😋"`),
		},
		"Nil data returned": {
			value:      []byte(`"give me 🚫"`),
			wantReturn: []byte(nil),
		},

		// Error cases
		"Error on empty data": {
			value:                        []byte{},
			wantError:                    ErrInvalidJSON,
			wantConvHandlerNotToBeCalled: true,
		},
		"Error on nil data": {
			value:                        nil,
			wantError:                    ErrInvalidJSON,
			wantConvHandlerNotToBeCalled: true,
		},
		"Error with empty data returned": {
			value:      []byte(`"give me 🗑‼"`),
			wantReturn: []byte{},
			wantError:  ErrInvalidJSON,
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			convFuncCalled := false
			mt := pam_test.NewModuleTransactionDummy(pam.BinaryPointerConversationFunc(
				func(ptr pam.BinaryPointer) (pam.BinaryPointer, error) {
					convFuncCalled = true
					require.NotNil(t, ptr)
					req, err := decodeJSONProtoMessage(ptr)
					require.NoError(t, err)
					require.Equal(t, tc.value, req)
					if tc.wantReturn != nil {
						msg, err := newJSONProtoMessage(tc.wantReturn)
						return pam.BinaryPointer(msg), err
					}
					msg, err := newJSONProtoMessage(req)
					return pam.BinaryPointer(msg), err
				}))

			data, err := sendToGdm(mt, tc.value)
			require.Equal(t, convFuncCalled, !tc.wantConvHandlerNotToBeCalled)

			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)

			if tc.wantReturn != nil {
				require.Equal(t, tc.wantReturn, data)
				return
			}
			require.Equal(t, tc.value, data)
		})
	}
}

func TestSendData(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		value *Data

		wantReturn                   []byte
		wantError                    error
		wantConvHandlerNotToBeCalled bool
	}{
		"Send data can handle null JSON value as return value": {
			value: &Data{
				Type: DataType_event,
				Event: &EventData{
					Type: EventType_brokerSelected,
					Data: &EventData_BrokerSelected{},
				},
			},

			wantReturn: []byte("null"),
		},
		"Can send Hello packet data": {
			value: &Data{
				Type:  DataType_hello,
				Hello: &HelloData{Version: 12345},
			},
			wantReturn: []byte(`"hello gdm!"`),
		},

		// Error cases
		"Error on empty data": {
			value:                        &Data{},
			wantConvHandlerNotToBeCalled: true,
			wantReturn:                   nil,
			wantError:                    errors.New("unexpected type unknownType"),
		},
		"Error on missing data return": {
			value: &Data{
				Type: DataType_event,
				Event: &EventData{
					Type: EventType_brokerSelected,
					Data: nil,
				},
			},

			wantConvHandlerNotToBeCalled: true,
			wantError:                    errors.New("missing event data"),
		},
		"Error on wrong data": {
			value: &Data{
				Type:    DataType_event,
				Request: &RequestData{},
			},
			wantConvHandlerNotToBeCalled: true,
			wantError:                    errors.New("missing event data"),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			convFuncCalled := false
			mtx := pam_test.NewModuleTransactionDummy(pam.BinaryPointerConversationFunc(
				func(ptr pam.BinaryPointer) (pam.BinaryPointer, error) {
					convFuncCalled = true
					require.NotNil(t, ptr)
					req, err := decodeJSONProtoMessage(ptr)
					require.NoError(t, err)
					valueJSON, err := tc.value.JSON()
					require.NoError(t, err)
					require.Equal(t, valueJSON, req)
					if tc.wantReturn != nil {
						msg, err := newJSONProtoMessage(tc.wantReturn)
						require.NoError(t, err)
						return pam.BinaryPointer(msg), nil
					}
					msg, err := newJSONProtoMessage(req)
					require.NoError(t, err)
					return pam.BinaryPointer(msg), nil
				}))

			data, err := sendData(mtx, tc.value)
			require.Equal(t, convFuncCalled, !tc.wantConvHandlerNotToBeCalled)

			if tc.wantError != nil {
				require.Nil(t, data)
				require.ErrorContains(t, err, tc.wantError.Error())
				return
			}
			require.NoError(t, err)

			if tc.wantReturn != nil {
				require.Equal(t, tc.wantReturn, data)
				return
			}
			require.Equal(t, tc.value, data)
		})
	}
}

func TestDataSendChecked(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		value *Data

		wantReturn                   *Data
		wantError                    error
		wantConvHandlerNotToBeCalled bool
	}{
		"Can send and receive Hello packet data": {
			value: &Data{
				Type:  DataType_hello,
				Hello: &HelloData{Version: 12345},
			},
			wantReturn: &Data{Type: DataType_hello},
		},
		"Can send event and receive an event ack": {
			value: &Data{
				Type: DataType_event,
				Event: &EventData{
					Type: EventType_brokerSelected,
					Data: &EventData_BrokerSelected{},
				},
			},

			wantReturn: &Data{Type: DataType_eventAck},
		},

		// Error cases
		"Error on empty data": {
			value:                        &Data{},
			wantConvHandlerNotToBeCalled: true,
			wantReturn:                   nil,
			wantError:                    errors.New("unexpected type unknownType"),
		},
		"Error on missing data return": {
			value: &Data{
				Type: DataType_event,
				Event: &EventData{
					Type: EventType_brokerSelected,
					Data: nil,
				},
			},
			wantConvHandlerNotToBeCalled: true,
			wantError:                    errors.New("missing event data"),
		},
		"Error on wrong data": {
			value: &Data{
				Type:    DataType_event,
				Request: &RequestData{},
			},
			wantConvHandlerNotToBeCalled: true,
			wantError:                    errors.New("missing event data"),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			convFuncCalled := false
			mtx := pam_test.NewModuleTransactionDummy(pam.BinaryPointerConversationFunc(
				func(ptr pam.BinaryPointer) (pam.BinaryPointer, error) {
					convFuncCalled = true
					require.NotNil(t, ptr)
					req, err := decodeJSONProtoMessage(ptr)
					require.NoError(t, err)
					valueJSON, err := tc.value.JSON()
					require.NoError(t, err)
					require.Equal(t, valueJSON, req)
					if tc.wantReturn != nil {
						bytes, err := tc.wantReturn.JSON()
						require.NoError(t, err)
						msg, err := newJSONProtoMessage(bytes)
						require.NoError(t, err)
						return pam.BinaryPointer(msg), nil
					}
					msg, err := newJSONProtoMessage(req)
					require.NoError(t, err)
					return pam.BinaryPointer(msg), nil
				}))

			data, err := SendData(mtx, tc.value)
			require.Equal(t, convFuncCalled, !tc.wantConvHandlerNotToBeCalled)

			if tc.wantError != nil {
				require.Nil(t, data)
				require.ErrorContains(t, err, tc.wantError.Error())
				return
			}

			require.NoError(t, err)

			if tc.wantReturn != nil {
				require.Equal(t, tc.wantReturn, data)
			} else {
				require.Equal(t, tc.value, data)
			}
		})
	}
}

func TestDataSendPoll(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		wantReturn                   *Data
		wantError                    error
		wantConvHandlerNotToBeCalled bool
	}{
		"Polling handles a null response": {
			wantReturn: &Data{
				Type: DataType_pollResponse,
			},
		},
		"Polling handles an empty response": {
			wantReturn: &Data{
				Type:         DataType_pollResponse,
				PollResponse: []*EventData{},
			},
		},
		"Polling handles multiple event events response": {
			wantReturn: &Data{
				Type: DataType_pollResponse,
				PollResponse: []*EventData{
					{Type: EventType_authEvent, Data: &EventData_AuthEvent{}},
					{Type: EventType_authModeSelected, Data: &EventData_AuthModeSelected{}},
					{Type: EventType_uiLayoutReceived, Data: &EventData_UiLayoutReceived{}},
				},
			},
		},

		// Error cases
		"Error on nil return": {
			wantReturn: nil,
			wantError:  errors.New("unexpected token null"),
		},
		"Error on unexpected type": {
			wantReturn: &Data{Type: DataType_hello},
			wantError:  errors.New("gdm replied with an unexpected type: hello"),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			convFuncCalled := false
			mtx := pam_test.NewModuleTransactionDummy(pam.BinaryPointerConversationFunc(
				func(ptr pam.BinaryPointer) (pam.BinaryPointer, error) {
					convFuncCalled = true
					require.NotNil(t, ptr)
					if tc.wantReturn != nil {
						bytes, err := tc.wantReturn.JSON()
						require.NoError(t, err)
						msg, err := newJSONProtoMessage(bytes)
						require.NoError(t, err)
						return pam.BinaryPointer(msg), nil
					}
					msg, err := newJSONProtoMessage([]byte("null"))
					require.NoError(t, err)
					return pam.BinaryPointer(msg), nil
				}))

			eventData, err := SendPoll(mtx)
			require.Equal(t, convFuncCalled, !tc.wantConvHandlerNotToBeCalled)

			if tc.wantError != nil {
				require.Nil(t, eventData)
				require.ErrorContains(t, err, tc.wantError.Error())
				return
			}
			require.NoError(t, err)
			requireEqualData(t, tc.wantReturn,
				&Data{Type: DataType_pollResponse, PollResponse: eventData})
		})
	}
}

func reformatJSONIndented(t *testing.T, input []byte) []byte {
	t.Helper()

	var indented bytes.Buffer
	err := json.Indent(&indented, input, "", "  ")
	require.NoError(t, err)
	return indented.Bytes()
}

func requireEqualData(t *testing.T, want *Data, actual *Data) {
	t.Helper()

	// We can't compare data values as their content may contain elements
	// that may vary that are needed by protobuf implementation.
	// So let's compare the data JSON representation instead since that's what
	// we care about anyways.
	wantJSON, err := want.JSON()
	require.NoError(t, err)
	actualJSON, err := actual.JSON()
	require.NoError(t, err)

	require.Equal(t, string(reformatJSONIndented(t, wantJSON)),
		string(reformatJSONIndented(t, actualJSON)))
}

type invalidRequest struct {
}

// Implement Request interface.
//
//nolint:revive // This is to implement Request interface defined by protobuf.
func (*invalidRequest) isRequestData_Data() {}

func TestDataSendRequestTyped(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		request Request

		wantData                     *Data
		wantError                    error
		wantConvHandlerNotToBeCalled bool
		wantReturnType               any
	}{
		"Request change state": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type: DataType_response,
				Response: &ResponseData{
					Type: RequestType_changeStage,
					Data: &ResponseData_Ack{},
				},
			},
		},
		"Request Ui layout capabilities": {
			request: &RequestData_UiLayoutCapabilities{},
			wantData: &Data{
				Type: DataType_response,
				Response: &ResponseData{
					Type: RequestType_uiLayoutCapabilities,
					Data: &ResponseData_UiLayoutCapabilities{},
				},
			},
		},
		"Request change state, handles nil response data": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type: DataType_response,
				Response: &ResponseData{
					Type: RequestType_changeStage,
					Data: (*ResponseData_Ack)(nil),
				},
			},
		},
		"Request Ui layout capabilities, handles nil response data": {
			request: &RequestData_UiLayoutCapabilities{},
			wantData: &Data{
				Type: DataType_response,
				Response: &ResponseData{
					Type: RequestType_uiLayoutCapabilities,
					Data: (*ResponseData_UiLayoutCapabilities)(nil),
				},
			},
		},
		"Request change state, expecting Ack type": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type: DataType_response,
				Response: &ResponseData{
					Type: RequestType_changeStage,
					Data: &ResponseData_Ack{},
				},
			},
			wantReturnType: &ResponseData_Ack{},
		},
		"Request Ui layout capabilities, expecting Ack type": {
			request: &RequestData_UiLayoutCapabilities{},
			wantData: &Data{
				Type: DataType_response,
				Response: &ResponseData{
					Type: RequestType_uiLayoutCapabilities,
					Data: &ResponseData_UiLayoutCapabilities{},
				},
			},
			wantError:      errors.New("impossible to convert"),
			wantReturnType: &ResponseData_Ack{},
		},
		"Request change state, handles nil response data, expecting Ack type": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type: DataType_response,
				Response: &ResponseData{
					Type: RequestType_changeStage,
					Data: (*ResponseData_Ack)(nil),
				},
			},
			wantReturnType: &ResponseData_Ack{},
		},
		"Request change state, expecting UiLayoutCapabilities type": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type: DataType_response,
				Response: &ResponseData{
					Type: RequestType_changeStage,
					Data: &ResponseData_Ack{},
				},
			},
			wantReturnType: &ResponseData_UiLayoutCapabilities{},
			wantError:      errors.New("impossible to convert"),
		},
		"Request Ui layout capabilities, handles nil response data, expecting UiLayoutCapabilities type": {
			request: &RequestData_UiLayoutCapabilities{},
			wantData: &Data{
				Type: DataType_response,
				Response: &ResponseData{
					Type: RequestType_uiLayoutCapabilities,
					Data: (*ResponseData_UiLayoutCapabilities)(nil),
				},
			},
			wantReturnType: &ResponseData_UiLayoutCapabilities{},
		},
		"Request Ui layout capabilities, expecting UiLayoutCapabilities type": {
			request: &RequestData_UiLayoutCapabilities{},
			wantData: &Data{
				Type: DataType_response,
				Response: &ResponseData{
					Type: RequestType_uiLayoutCapabilities,
					Data: &ResponseData_UiLayoutCapabilities{
						UiLayoutCapabilities: &Responses_UiLayoutCapabilities{
							SupportedUiLayouts: []*authd.UILayout{
								{
									Type: "form",
								},
							},
						},
					},
				},
			},
			wantReturnType: &ResponseData_UiLayoutCapabilities{},
		},

		// Error cases
		"Error with unknown request": {
			request:                      &invalidRequest{},
			wantConvHandlerNotToBeCalled: true,
			wantError:                    errors.New("no known request type"),
		},
		// Error cases
		"Error on nil return": {
			request:   &RequestData_ChangeStage{},
			wantData:  nil,
			wantError: errors.New("unexpected token null"),
		},
		"Error with mismatching response type": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type:     DataType_response,
				Response: &ResponseData{Type: RequestType_uiLayoutCapabilities},
			},
			wantError: errors.New("gdm replied with invalid response type"),
		},
		"Error with non-response type": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type: DataType_hello,
			},
			wantError: errors.New("gdm replied with an unexpected type: hello"),
		},
		"Error with unknown request expecting Ack type": {
			request:                      &invalidRequest{},
			wantConvHandlerNotToBeCalled: true,
			wantError:                    errors.New("no known request type"),
			wantReturnType:               &ResponseData_Ack{},
		},
		"Error with mismatching response type expecting Ack type": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type:     DataType_response,
				Response: &ResponseData{Type: RequestType_uiLayoutCapabilities},
			},
			wantError:      errors.New("gdm replied with invalid response type"),
			wantReturnType: &ResponseData_Ack{},
		},
		"Error with non-response type expecting Ack type": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type: DataType_hello,
			},
			wantError:      errors.New("gdm replied with an unexpected type: hello"),
			wantReturnType: &ResponseData_Ack{},
		},
		"Error with unknown request expecting UiLayoutCapabilities type": {
			request:                      &invalidRequest{},
			wantConvHandlerNotToBeCalled: true,
			wantError:                    errors.New("no known request type"),
			wantReturnType:               &ResponseData_UiLayoutCapabilities{},
		},
		"Error with mismatching response type expecting UiLayoutCapabilities type": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type:     DataType_response,
				Response: &ResponseData{Type: RequestType_uiLayoutCapabilities},
			},
			wantError:      errors.New("gdm replied with invalid response type"),
			wantReturnType: &ResponseData_UiLayoutCapabilities{},
		},
		"Error with non-response type expecting UiLayoutCapabilities type": {
			request: &RequestData_ChangeStage{},
			wantData: &Data{
				Type: DataType_hello,
			},
			wantError:      errors.New("gdm replied with an unexpected type: hello"),
			wantReturnType: &ResponseData_UiLayoutCapabilities{},
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			convFuncCalled := false
			mtx := pam_test.NewModuleTransactionDummy(pam.BinaryPointerConversationFunc(
				func(ptr pam.BinaryPointer) (pam.BinaryPointer, error) {
					convFuncCalled = true
					require.NotNil(t, ptr)
					if tc.wantData != nil {
						bytes, err := tc.wantData.JSON()
						require.NoError(t, err)
						msg, err := newJSONProtoMessage(bytes)
						require.NoError(t, err)
						return pam.BinaryPointer(msg), nil
					}
					msg, err := newJSONProtoMessage([]byte("null"))
					require.NoError(t, err)
					return pam.BinaryPointer(msg), nil
				}))
			var response Response
			var err error
			switch tc.wantReturnType.(type) {
			case *ResponseData_UiLayoutCapabilities:
				response, err = SendRequestTyped[*ResponseData_UiLayoutCapabilities](mtx, tc.request)
			case *ResponseData_Ack:
				response, err = SendRequestTyped[*ResponseData_Ack](mtx, tc.request)
			default:
				response, err = SendRequestTyped[Response](mtx, tc.request)
			}
			require.Equal(t, convFuncCalled, !tc.wantConvHandlerNotToBeCalled)

			if tc.wantError != nil {
				require.Nil(t, response)
				require.ErrorContains(t, err, tc.wantError.Error())
				return
			}
			require.NoError(t, err)
			requireEqualData(t, tc.wantData, &Data{
				Type:     DataType_response,
				Response: &ResponseData{Type: tc.wantData.Response.Type, Data: response},
			})
		})
	}
}

type invalidEvent struct {
}

// Implement Event interface.
//
//nolint:revive // This is to implement Request interface defined by protobuf.
func (*invalidEvent) isEventData_Data() {}

func TestDataEmitEvent(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		event        Event
		returnedData []byte

		wantEventType                EventType
		wantError                    error
		wantConvHandlerNotToBeCalled bool
	}{
		"Emit event BrokersReceived": {
			event:         &EventData_BrokersReceived{},
			wantEventType: EventType_brokersReceived,
		},
		"Emit event BrokerSelected": {
			event:         &EventData_BrokerSelected{},
			wantEventType: EventType_brokerSelected,
		},
		"Emit event AuthModesReceived": {
			event:         &EventData_AuthModesReceived{},
			wantEventType: EventType_authModesReceived,
		},
		"Emit event AuthModeSelected": {
			event:         &EventData_AuthModeSelected{},
			wantEventType: EventType_authModeSelected,
		},
		"Emit event IsAuthenticatedRequested": {
			event:         &EventData_IsAuthenticatedRequested{},
			wantEventType: EventType_isAuthenticatedRequested,
		},
		"Emit event StageChanged": {
			event:         &EventData_StageChanged{},
			wantEventType: EventType_stageChanged,
		},
		"Emit event UiLayoutReceived": {
			event:         &EventData_UiLayoutReceived{},
			wantEventType: EventType_uiLayoutReceived,
		},
		"Emit event AuthEvent": {
			event:         &EventData_AuthEvent{},
			wantEventType: EventType_authEvent,
		},
		"Emit event ReselectAuthMode": {
			event:         &EventData_ReselectAuthMode{},
			wantEventType: EventType_reselectAuthMode,
		},

		// Error cases
		"Error on nil event": {
			wantConvHandlerNotToBeCalled: true,
			wantError:                    errors.New("no known event type"),
		},
		"Error on unexpected event type": {
			event:                        &invalidEvent{},
			wantConvHandlerNotToBeCalled: true,
			wantError:                    fmt.Errorf("no known event type %#v", &invalidEvent{}),
		},
		"Error on invalid data": {
			event:        &EventData_ReselectAuthMode{},
			returnedData: []byte("null"),
			wantError:    errors.New("unexpected token null"),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			convFuncCalled := false
			mtx := pam_test.NewModuleTransactionDummy(pam.BinaryPointerConversationFunc(
				func(ptr pam.BinaryPointer) (pam.BinaryPointer, error) {
					convFuncCalled = true
					require.NotNil(t, ptr)
					if tc.returnedData != nil {
						msg, err := newJSONProtoMessage(tc.returnedData)
						require.NoError(t, err)
						return pam.BinaryPointer(msg), nil
					}

					jsonReq, err := decodeJSONProtoMessage(ptr)
					require.NoError(t, err)
					data, err := NewDataFromJSON(jsonReq)
					require.NoError(t, err)
					require.Equal(t, data.Type, DataType_event)
					require.Equal(t, data.Event.Type, tc.wantEventType)
					json, err := (&Data{Type: DataType_eventAck}).JSON()
					require.NoError(t, err)
					msg, err := newJSONProtoMessage(json)
					require.NoError(t, err)
					return pam.BinaryPointer(msg), nil
				}))

			err := EmitEvent(mtx, tc.event)
			require.Equal(t, convFuncCalled, !tc.wantConvHandlerNotToBeCalled)

			if tc.wantError != nil {
				require.ErrorContains(t, err, tc.wantError.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}
