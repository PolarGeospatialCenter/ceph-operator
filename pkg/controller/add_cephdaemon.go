package controller

import (
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/cephdaemon"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, cephdaemon.Add)
}
