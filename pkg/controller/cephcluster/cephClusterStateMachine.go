package cephcluster

import (
	"context"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TransitionFunc func(client.Client, *runtime.Scheme) error

type CephClusterStateMachine interface {
	State() cephv1alpha1.CephClusterState
	GetTransition(ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephClusterState)
}

type ReadOnlyClient interface {
	Get(context.Context, types.NamespacedName, runtime.Object) error
	List(context.Context, *client.ListOptions, runtime.Object) error
}

func NewCephClusterStateMachine(cluster *cephv1alpha1.CephCluster, logger logr.Logger) CephClusterStateMachine {

	return newBaseStateMachine(cluster, logger)
}

func newBaseStateMachine(cluster *cephv1alpha1.CephCluster, logger logr.Logger) *BaseStateMachine {
	return &BaseStateMachine{cluster: cluster, logger: logger}
}

type BaseStateMachine struct {
	cluster *cephv1alpha1.CephCluster
	logger  logr.Logger
}

func (s *BaseStateMachine) clusterEnabled() bool {
	return !s.cluster.Spec.Disabled
}

func (s *BaseStateMachine) State() cephv1alpha1.CephClusterState {
	return s.cluster.GetState()
}

func (s *BaseStateMachine) logError(client client.Client, scheme *runtime.Scheme) error {
	s.logger.Info("ceph cluster is in error state")
	return nil
}

func (s *BaseStateMachine) emitError(err error) TransitionFunc {

	return TransitionFunc(func(_ client.Client, _ *runtime.Scheme) error {
		s.logger.Error(err, "")
		return nil
	})
}

func (s *BaseStateMachine) listMonCluster(readClient ReadOnlyClient) (*cephv1alpha1.CephMonClusterList, error) {
	monClusterList := &cephv1alpha1.CephMonClusterList{}
	monClusterListOptions := &client.ListOptions{}
	monClusterListOptions.MatchingLabels(map[string]string{
		cephv1alpha1.ClusterNameLabel: s.cluster.GetName(),
	})

	return monClusterList, readClient.List(context.TODO(), monClusterListOptions, monClusterList)
}

func (s *BaseStateMachine) listDaemonCluster(readClient ReadOnlyClient) (*cephv1alpha1.CephDaemonClusterList, error) {
	daemonClusterList := &cephv1alpha1.CephDaemonClusterList{}
	daemonClusterListOptions := &client.ListOptions{}
	daemonClusterListOptions.MatchingLabels(map[string]string{
		cephv1alpha1.ClusterNameLabel: s.cluster.GetName(),
	})

	return daemonClusterList, readClient.List(context.TODO(), daemonClusterListOptions, daemonClusterList)
}

func (s *BaseStateMachine) monClusterInQuorum(readClient ReadOnlyClient) (bool, error) {

	monList, err := s.listMonCluster(readClient)
	if err != nil {
		return false, err
	}

	return monList.AllInState(cephv1alpha1.MonClusterInQuorum), nil
}

func (s *BaseStateMachine) daemonClustersRunning(readClient ReadOnlyClient) (bool, error) {
	daemonList, err := s.listDaemonCluster(readClient)
	if err != nil {
		return false, err
	}

	return daemonList.AllInState(cephv1alpha1.CephDaemonClusterStateRunning), nil
}

func (s *BaseStateMachine) osdsRunning(readClient ReadOnlyClient) (bool, error) {
	return true, nil
}

func (s *BaseStateMachine) daemonClustersIdle(readClient ReadOnlyClient) (bool, error) {

	daemonList, err := s.listDaemonCluster(readClient)
	if err != nil {
		return false, err
	}

	return daemonList.AllInState(cephv1alpha1.CephDaemonClusterStateIdle), nil
}

func (s *BaseStateMachine) osdsIdle(readClient ReadOnlyClient) (bool, error) {

	return true, nil
}

func (s *BaseStateMachine) monClusterIdle(readClient ReadOnlyClient) (bool, error) {
	monList, err := s.listMonCluster(readClient)
	if err != nil {
		return false, err
	}

	return monList.AllInState(cephv1alpha1.MonClusterIdle), nil
}

func (s *BaseStateMachine) GetTransition(readClient ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephClusterState) {

	switch s.State() {
	case cephv1alpha1.CephClusterIdle:
		if s.clusterEnabled() {
			return nil, cephv1alpha1.CephClusterStartMons
		}
	case cephv1alpha1.CephClusterStartMons:
		if !s.clusterEnabled() {
			return nil, cephv1alpha1.CephClusterShutdown
		}
		return s.ifReady(readClient, s.monClusterInQuorum, cephv1alpha1.CephClusterStartDaemons)

	case cephv1alpha1.CephClusterStartDaemons:
		if !s.clusterEnabled() {
			return nil, cephv1alpha1.CephClusterShutdown
		}
		return s.ifReady(readClient, s.daemonClustersRunning, cephv1alpha1.CephClusterStartOsds)

	case cephv1alpha1.CephClusterStartOsds:
		if !s.clusterEnabled() {
			return nil, cephv1alpha1.CephClusterShutdown
		}
		return s.ifReady(readClient, s.osdsRunning, cephv1alpha1.CephClusterRunning)

	case cephv1alpha1.CephClusterRunning:
		if !s.clusterEnabled() {
			return nil, cephv1alpha1.CephClusterShutdown
		}

	case cephv1alpha1.CephClusterShutdown:
		return nil, cephv1alpha1.CephClusterStopDaemons

	case cephv1alpha1.CephClusterStopDaemons:
		return s.ifReady(readClient, s.daemonClustersIdle, cephv1alpha1.CephClusterStopOsds)

	case cephv1alpha1.CephClusterStopOsds:
		return s.ifReady(readClient, s.osdsIdle, cephv1alpha1.CephClusterStopMons)

	case cephv1alpha1.CephClusterStopMons:
		return s.ifReady(readClient, s.monClusterIdle, cephv1alpha1.CephClusterIdle)

	default:
		return nil, cephv1alpha1.CephClusterShutdown
	}

	return nil, s.State()
}

type readyCheck func(ReadOnlyClient) (bool, error)

func (s *BaseStateMachine) ifReady(readClient ReadOnlyClient, ready readyCheck,
	nextState cephv1alpha1.CephClusterState) (TransitionFunc, cephv1alpha1.CephClusterState) {

	isReady, err := ready(readClient)
	if err != nil {
		return s.emitError(err), s.State()
	}
	if isReady {
		return nil, nextState
	}

	return nil, s.State()
}
