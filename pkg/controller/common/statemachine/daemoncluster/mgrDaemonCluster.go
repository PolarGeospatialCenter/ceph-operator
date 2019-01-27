package daemoncluster

import (
	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
)

type MgrStateMachine struct {
	*BaseStateMachine
}

func (s *MgrStateMachine) GetTransition(client ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonClusterState) {

	switch s.State() {
	default:
		return s.BaseStateMachine.GetTransition(client)
	}
}
