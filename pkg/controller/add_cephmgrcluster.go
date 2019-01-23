package controller

import (
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/cephmgrcluster"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, cephmgrcluster.Add)
}
