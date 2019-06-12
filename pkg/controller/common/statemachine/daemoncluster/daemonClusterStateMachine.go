package daemoncluster

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common/statemachine"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewStateMachine(daemonCluster statemachine.DaemonCluster,
	cluster *cephv1alpha1.CephCluster, logger logr.Logger) CephDaemonClusterStateMachine {

	switch daemonCluster.GetDaemonType() {
	case cephv1alpha1.CephDaemonTypeMgr:
		return &MgrStateMachine{BaseStateMachine: NewBaseStateMachine(daemonCluster, cluster, logger)}
	case cephv1alpha1.CephDaemonTypeMds:
		return &MdsStateMachine{BaseStateMachine: NewBaseStateMachine(daemonCluster, cluster, logger)}
	case cephv1alpha1.CephDaemonTypeMon:
		return &MonClusterStateMachine{BaseStateMachine: NewBaseStateMachine(daemonCluster, cluster, logger)}
	default:
		return nil
	}
}

func NewBaseStateMachine(daemonCluster statemachine.DaemonCluster,
	cluster *cephv1alpha1.CephCluster, logger logr.Logger) *BaseStateMachine {
	return &BaseStateMachine{cluster: cluster, daemonCluster: daemonCluster, logger: logger}
}

type BaseStateMachine struct {
	cluster       *cephv1alpha1.CephCluster
	daemonCluster statemachine.DaemonCluster
	logger        logr.Logger
}

func (s *BaseStateMachine) daemonClusterEnabled() bool {
	return !s.daemonCluster.Disabled() && s.cluster.GetDaemonEnabled(s.daemonCluster.GetDaemonType())
}

func (s *BaseStateMachine) State() cephv1alpha1.CephDaemonClusterState {
	return s.daemonCluster.GetState()
}

func (s *BaseStateMachine) logError(client client.Client, scheme *runtime.Scheme) error {
	s.logger.Info("ceph daemon is in error state")
	return nil
}

func (s *BaseStateMachine) emitError(err error) statemachine.TransitionFunc {

	return statemachine.TransitionFunc(func(_ client.Client, _ *runtime.Scheme) error {
		s.logger.Error(err, "")
		return nil
	})
}

func (s *BaseStateMachine) scaleDown(client client.Client, scheme *runtime.Scheme) error {
	daemons, err := s.listDaemons(client)
	if err != nil {
		return err
	}

	if len(daemons.Items) < 1 {
		return fmt.Errorf("unabled to delete daemon, no daemons found")
	}

	var toDelete cephv1alpha1.CephDaemon

	rand.Seed(time.Now().UnixNano())
	toDelete = daemons.Items[rand.Int()%len(daemons.Items)]

	return client.Delete(context.TODO(), &toDelete)
}

func (s *BaseStateMachine) scaleUp(client client.Client, scheme *runtime.Scheme) error {
	daemon := cephv1alpha1.NewCephDaemon(s.daemonCluster.GetDaemonType(), s.daemonCluster.GetCephClusterName())

	daemon.Spec.Image = s.daemonCluster.GetImage()
	daemon.Spec.CephConfConfigMapName = s.cluster.GetCephConfigMapName()
	daemon.Namespace = s.daemonCluster.GetNamespace()

	if err := controllerutil.SetControllerReference(s.daemonCluster, daemon, scheme); err != nil {
		return err
	}

	err := client.Create(context.TODO(), daemon)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (s *BaseStateMachine) listDaemons(readClient statemachine.ReadOnlyClient) (*cephv1alpha1.CephDaemonList, error) {
	daemonList := &cephv1alpha1.CephDaemonList{}
	daemonListOptions := &client.ListOptions{}
	daemonListOptions.MatchingLabels(map[string]string{
		cephv1alpha1.ClusterNameLabel: s.cluster.GetName(),
		cephv1alpha1.DaemonTypeLabel:  s.daemonCluster.GetDaemonType().String(),
	})

	return daemonList, readClient.List(context.TODO(), daemonListOptions, daemonList)
}

func (s *BaseStateMachine) correctReplicaCount(client statemachine.ReadOnlyClient) (bool, error) {
	daemons, err := s.listDaemons(client)
	if err != nil {
		return false, err
	}
	return len(daemons.Items) == s.daemonCluster.DesiredReplicas(), nil
}

func (s *BaseStateMachine) scale(client statemachine.ReadOnlyClient) (statemachine.TransitionFunc, error) {

	daemons, err := s.listDaemons(client)
	if err != nil {
		return nil, err
	}
	daemonCount := len(daemons.Items)
	if daemonCount > s.daemonCluster.DesiredReplicas() {
		return s.scaleDown, nil
	}

	if daemonCount < s.daemonCluster.DesiredReplicas() {
		return s.scaleUp, nil
	}

	return nil, nil
}

func (s *BaseStateMachine) GetTransition(client statemachine.ReadOnlyClient) (statemachine.TransitionFunc, cephv1alpha1.CephDaemonClusterState) {

	if !s.daemonClusterEnabled() {
		return nil, cephv1alpha1.CephDaemonClusterStateIdle
	}

	switch s.State() {
	case cephv1alpha1.CephDaemonClusterStateIdle:
		if s.daemonClusterEnabled() {
			return nil, cephv1alpha1.CephDaemonClusterStateRunning
		}
	case cephv1alpha1.CephDaemonClusterStateRunning:
		correctCount, err := s.correctReplicaCount(client)
		if err != nil {
			return s.emitError(err), cephv1alpha1.CephDaemonClusterStateError
		}

		if !correctCount {
			return nil, cephv1alpha1.CephDaemonClusterStateScaling
		}

	case cephv1alpha1.CephDaemonClusterStateScaling:
		scaleFunc, err := s.scale(client)
		if err != nil {
			return s.emitError(err), cephv1alpha1.CephDaemonClusterStateError
		}

		if scaleFunc != nil {
			return scaleFunc, cephv1alpha1.CephDaemonClusterStateScaling
		}
		return nil, cephv1alpha1.CephDaemonClusterStateRunning

	case cephv1alpha1.CephDaemonClusterStateError:
		return nil, cephv1alpha1.CephDaemonClusterStateRunning

	default:
		return nil, cephv1alpha1.CephDaemonClusterStateIdle
	}

	return nil, s.State()
}
