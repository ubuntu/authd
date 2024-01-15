package gdm

import "C"

import (
	"errors"

	"github.com/msteinert/pam/v2"
)

func sendToGdm(pamMTx pam.ModuleTransaction, data []byte) ([]byte, error) {
	binReq, err := NewBinaryJSONProtoRequest(data)
	if err != nil {
		return nil, err
	}
	defer binReq.Release()
	res, err := pamMTx.StartConv(binReq)
	if err != nil {
		return nil, err
	}

	binRes, ok := res.(pam.BinaryConvResponse)
	if !ok {
		return nil, errors.New("returned value is not in binary form")
	}
	defer binRes.Release()
	return binRes.Decode(decodeJSONProtoMessage)
}
