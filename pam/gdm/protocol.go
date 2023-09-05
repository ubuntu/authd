//go:generate ../../tools/generate-proto.sh -I../.. gdm.proto

// Package gdm is the package for the GDM pam module handing.
package gdm

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"google.golang.org/protobuf/encoding/protojson"
)

const (
	// ProtoVersion is the version of the JSON protocol.
	ProtoVersion = uint32(1)
)

// Request is an interface implementing all the gdm requests.
type Request = isRequestData_Data

// Response is an interface implementing all the gdm responses.
type Response = isResponseData_Data

// Event is an interface implementing all the gdm events.
type Event = isEventData_Data

// NewDataFromJSON unmarshals data from json bytes.
func NewDataFromJSON(bytes []byte) (*Data, error) {
	var gdmData Data
	if err := protojson.Unmarshal(bytes, &gdmData); err != nil {
		return nil, err
	}

	if err := gdmData.Check(); err != nil {
		return nil, err
	}

	return &gdmData, nil
}

// FIXME: Do not do this when building the module in release mode, this is
// just relevant for testing purposes.
func (d *Data) checkMembers(acceptedMembers []string) error {
	//nolint:govet //We only redirect the value to figure out its type.
	val := reflect.ValueOf(*d)
	typ := val.Type()
	acceptedMembers = append(acceptedMembers, []string{
		"Type", "state", "sizeCache", "unknownFields",
	}...)

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		if slices.Contains(acceptedMembers, fieldType.Name) {
			continue
		}

		if !field.IsZero() {
			return fmt.Errorf("field %v should not be defined", fieldType.Name)
		}
	}

	return nil
}

// Check allows to check the sanity of a data value.
func (d *Data) Check() error {
	switch d.Type {
	case DataType_unknownType:
		return fmt.Errorf("unexpected type %v", d.Type.String())

	case DataType_hello:
		if err := d.checkMembers([]string{"Hello"}); err != nil {
			return err
		}

	case DataType_event:
		if d.Event == nil {
			return fmt.Errorf("missing event data")
		}
		if d.Event.Type == EventType_unknownEvent {
			return errors.New("missing event type")
		}
		if _, ok := EventType_name[int32(d.Event.Type)]; !ok {
			return fmt.Errorf("unexpected event type %v", d.Event.Type)
		}
		if d.Event.Data == nil {
			return fmt.Errorf("missing event data")
		}
		if err := d.checkMembers([]string{"Event"}); err != nil {
			return err
		}

	case DataType_eventAck:
		if err := d.checkMembers([]string{}); err != nil {
			return err
		}

	case DataType_request:
		if d.Request == nil {
			return errors.New("missing request data")
		}
		if d.Request.Type == RequestType_unknownRequest {
			return errors.New("missing request type")
		}
		if _, ok := RequestType_name[int32(d.Request.Type)]; !ok {
			return fmt.Errorf("unexpected request type %v", d.Request.Type)
		}
		if err := d.checkMembers([]string{"Request"}); err != nil {
			return err
		}

	case DataType_response:
		if d.Response == nil {
			return errors.New("missing response data")
		}
		if d.Response.Type == RequestType_unknownRequest {
			return errors.New("missing response type")
		}
		if _, ok := RequestType_name[int32(d.Response.Type)]; !ok {
			return fmt.Errorf("unexpected request type %v", d.Response.Type)
		}
		if err := d.checkMembers([]string{"Response"}); err != nil {
			return err
		}

	case DataType_poll:
		if err := d.checkMembers([]string{}); err != nil {
			return err
		}

	case DataType_pollResponse:
		if err := d.checkMembers([]string{"PollResponse"}); err != nil {
			return err
		}
		for i, response := range d.PollResponse {
			data := &Data{Type: DataType_event, Event: response}
			if err := data.Check(); err != nil {
				return fmt.Errorf("poll response data member %v invalid: %v", i, err)
			}
		}

	default:
		return fmt.Errorf("unhandled type %v", d.Type)
	}

	return nil
}

// JSON returns the data object serialized as JSON bytes.
func (d *Data) JSON() ([]byte, error) {
	bytes, err := protojson.Marshal(d)
	if err != nil {
		return nil, err
	}

	if err = d.Check(); err != nil {
		return nil, err
	}

	return bytes, err
}
