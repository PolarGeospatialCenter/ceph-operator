package cephdaemoncluster

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type TransitionFunc func(client.Client, *runtime.Scheme) error

//type podCheckFunc func(*corev1.Pod) bool

type CephDaemonClusterStateMachine interface {
	State() cephv1alpha1.CephDaemonClusterState
	GetTransition(ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonClusterState)
}

type ReadOnlyClient interface {
	Get(context.Context, types.NamespacedName, runtime.Object) error
	List(context.Context, *client.ListOptions, runtime.Object) error
}

func NewCephDaemonClusterStateMachine(daemonCluster *cephv1alpha1.CephDaemonCluster,
	cluster *cephv1alpha1.CephCluster, logger logr.Logger) CephDaemonClusterStateMachine {

	switch daemonCluster.Spec.DaemonType {
	case cephv1alpha1.CephDaemonTypeMgr:
		return &MgrStateMachine{BaseStateMachine: newBaseStateMachine(daemonCluster, cluster, logger)}
	case cephv1alpha1.CephDaemonTypeMds:
		return &MdsStateMachine{BaseStateMachine: newBaseStateMachine(daemonCluster, cluster, logger)}
	default:
		return nil
	}
}

func newBaseStateMachine(daemonCluster *cephv1alpha1.CephDaemonCluster,
	cluster *cephv1alpha1.CephCluster, logger logr.Logger) *BaseStateMachine {
	return &BaseStateMachine{cluster: cluster, daemonCluster: daemonCluster, logger: logger}
}

type BaseStateMachine struct {
	cluster       *cephv1alpha1.CephCluster
	daemonCluster *cephv1alpha1.CephDaemonCluster
	logger        logr.Logger
}

func (s *BaseStateMachine) daemonClusterEnabled() bool {
	return !s.daemonCluster.Spec.Disabled && s.cluster.GetDaemonEnabled(s.daemonCluster.GetDaemonType())
}

func (s *BaseStateMachine) State() cephv1alpha1.CephDaemonClusterState {
	return s.daemonCluster.GetState()
}

func (s *BaseStateMachine) logError(client client.Client, scheme *runtime.Scheme) error {
	s.logger.Info("ceph daemon is in error state")
	return nil
}

func (s *BaseStateMachine) emitError(err error) TransitionFunc {

	return TransitionFunc(func(_ client.Client, _ *runtime.Scheme) error {
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
	daemon.Spec.CephConfConfigMapName = s.daemonCluster.GetCephConfConfigMapName()
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

func (s *BaseStateMachine) listDaemons(readClient ReadOnlyClient) (*cephv1alpha1.CephDaemonList, error) {
	daemonList := &cephv1alpha1.CephDaemonList{}
	daemonListOptions := &client.ListOptions{}
	daemonListOptions.MatchingLabels(map[string]string{
		cephv1alpha1.ClusterNameLabel: s.cluster.GetName(),
		cephv1alpha1.DaemonTypeLabel:  s.daemonCluster.GetDaemonType().String(),
	})

	return daemonList, readClient.List(context.TODO(), daemonListOptions, daemonList)
}

func (s *BaseStateMachine) correctReplicaCount(client ReadOnlyClient) (bool, error) {
	daemons, err := s.listDaemons(client)
	if err != nil {
		return false, err
	}
	return len(daemons.Items) == s.daemonCluster.Spec.Replicas, nil
}

func (s *BaseStateMachine) scale(client ReadOnlyClient) (TransitionFunc, error) {

	daemons, err := s.listDaemons(client)
	if err != nil {
		return nil, err
	}
	daemonCount := len(daemons.Items)
	if daemonCount > s.daemonCluster.Spec.Replicas {
		return s.scaleDown, nil
	}

	if daemonCount < s.daemonCluster.Spec.Replicas {
		return s.scaleUp, nil
	}

	return nil, nil
}

func (s *BaseStateMachine) GetTransition(client ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonClusterState) {

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

type MgrStateMachine struct {
	*BaseStateMachine
}

func (s *MgrStateMachine) GetTransition(client ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonClusterState) {

	switch s.State() {
	default:
		return s.BaseStateMachine.GetTransition(client)
	}
}

type MdsStateMachine struct {
	*BaseStateMachine
}

func (s *MdsStateMachine) GetTransition(client ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonClusterState) {

	switch s.State() {
	default:
		return s.BaseStateMachine.GetTransition(client)
	}
}
