//go:build withgdmmodel

package adapter

import "github.com/msteinert/pam/v2"

func getGdmModel(pamMTx pam.ModuleTransaction) gdmModeler {
	return gdmModel{pamMTx: pamMTx}
}
