package daemoncluster

import (
	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common/statemachine"
)

type CephDaemonClusterStateMachine interface {
	State() cephv1alpha1.CephDaemonClusterState
	GetTransition(statemachine.ReadOnlyClient) (statemachine.TransitionFunc, cephv1alpha1.CephDaemonClusterState)
}
