package daemoncluster

import (
	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common/statemachine"
)

type MdsStateMachine struct {
	*BaseStateMachine
}

func (s *MdsStateMachine) GetTransition(client statemachine.ReadOnlyClient) (statemachine.TransitionFunc, cephv1alpha1.CephDaemonClusterState) {

	switch s.State() {
	default:
		return s.BaseStateMachine.GetTransition(client)
	}
}
