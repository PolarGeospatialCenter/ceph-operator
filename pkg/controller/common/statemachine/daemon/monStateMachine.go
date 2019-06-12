package daemon

import (
	"context"
	"fmt"
	"net"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common/statemachine"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type MonStateMachine struct {
	*BaseStateMachine
}

func (s *MonStateMachine) GetTransition(readClient statemachine.ReadOnlyClient) (statemachine.TransitionFunc, cephv1alpha1.CephDaemonState) {
	// Check for disabled or lost quorum states
	if (s.daemon.GetDisabled() ||
		s.daemonCluster.GetState() == cephv1alpha1.MonClusterLostQuorum ||
		s.daemonCluster.GetState() == cephv1alpha1.MonClusterIdle) &&
		!(s.daemon.GetState() == cephv1alpha1.MonCleanup ||
			s.daemon.GetState() == cephv1alpha1.MonIdle) {

		return nil, cephv1alpha1.MonCleanup
	}

	switch s.daemon.GetState() {
	case cephv1alpha1.MonError:
		s.BaseStateMachine.logger.Info("Monitor is in error state, cleaning up", "MonitorID", s.daemon.GetDaemonID())
		return nil, cephv1alpha1.MonCleanup

	case cephv1alpha1.MonCleanup:
		return s.deletePod, cephv1alpha1.MonIdle

	case cephv1alpha1.MonIdle:
		if s.daemon.GetDisabled() {
			return nil, cephv1alpha1.MonIdle
		}
		switch s.daemonCluster.GetState() {
		case cephv1alpha1.MonClusterInQuorum:
			return nil, cephv1alpha1.MonLaunchPod

		case cephv1alpha1.MonClusterLaunching:
			if s.daemon.Status.InitalMember {
				return nil, cephv1alpha1.MonLaunchPod
			}

		default:
			return nil, s.State()
		}

	case cephv1alpha1.MonLaunchPod:
		if !s.daemonCluster.StateIn(cephv1alpha1.MonClusterInQuorum,
			cephv1alpha1.MonClusterEstablishingQuorum,
			cephv1alpha1.MonClusterLaunching) {
			s.BaseStateMachine.logger.Info("Refusing to launch monitor while cluster is unexpected state",
				"ClusterState", s.daemonCluster.GetState(), "MonitorId", s.daemon.GetDaemonID())
			return nil, cephv1alpha1.MonLaunchPod
		}

		switch s.daemonCluster.GetState() {
		case cephv1alpha1.MonClusterInQuorum:
			return s.launchPod, cephv1alpha1.MonWaitForPodReady
		case cephv1alpha1.MonClusterLaunching:
			return s.launchPod, cephv1alpha1.MonWaitForPodRun
		case cephv1alpha1.MonClusterEstablishingQuorum:
			return s.launchPod, cephv1alpha1.MonWaitForPodRun
		}

	case cephv1alpha1.MonWaitForPodRun:
		running, err := s.checkPod(readClient, podRunning)
		if err != nil {
			return nil, cephv1alpha1.MonError
		}
		if running {
			return s.updateMonStatus(false), cephv1alpha1.MonWaitForPodReady
		}

	case cephv1alpha1.MonWaitForPodReady:
		ready, err := s.checkPod(readClient, podReady)
		if err != nil {
			return nil, cephv1alpha1.MonError
		}
		if ready {
			return s.updateMonStatus(true), cephv1alpha1.MonInQuorum
		}

	case cephv1alpha1.MonInQuorum:
		quorum, err := s.checkPod(readClient, podReady)

		if errors.IsNotFound(err) {
			return nil, cephv1alpha1.MonError
		}

		if quorum || err != nil {
			return nil, cephv1alpha1.MonInQuorum
		}

		// out of quorum with no error
		return nil, cephv1alpha1.MonCleanup

	default:
		return nil, cephv1alpha1.MonCleanup
	}
	return nil, s.State()
}

func (s *MonStateMachine) updateMonStatus(setInitialMember bool) statemachine.TransitionFunc {

	return func(c client.Client, scheme *runtime.Scheme) error {
		pod := &corev1.Pod{}
		err := c.Get(context.TODO(), types.NamespacedName{
			Name:      s.daemon.GetPodName(),
			Namespace: s.daemon.GetNamespace(),
		}, pod)
		if err != nil {
			return err
		}

		instance := s.daemon.(*cephv1alpha1.CephMon)
		monCluster := s.daemon.(*cephv1alpha1.CephMonCluster)

		instance.Status.PodIP = net.ParseIP(pod.Status.PodIP)
		instance.Status.StartEpoch = monCluster.Status.StartEpoch
		if setInitialMember {
			instance.Status.InitalMember = true
		}
		return c.Update(context.TODO(), instance)
	}
}

func (s *MonStateMachine) launchPod(c client.Client, scheme *runtime.Scheme) error {
	// Create PVC
	pvc, err := s.daemon.GetVolumeClaimTemplate()
	if err != nil {
		return err
	}
	pvc.Namespace = s.daemon.GetNamespace()

	if err := controllerutil.SetControllerReference(s.daemon, pvc, scheme); err != nil {
		return err
	}

	err = c.Create(context.TODO(), pvc)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	// Create Pod
	adminKeyringSecretList := &corev1.SecretList{}
	listOptions := &client.ListOptions{}
	listOptions.MatchingLabels(map[string]string{
		cephv1alpha1.ClusterNameLabel:   s.daemon.GetCephClusterName(),
		cephv1alpha1.KeyringEntityLabel: "client.admin",
	})

	err = c.List(context.TODO(), listOptions, adminKeyringSecretList)
	if err != nil {
		return err
	}
	if len(adminKeyringSecretList.Items) != 1 {
		return fmt.Errorf("expecting unique client admin keyring: found %d", len(adminKeyringSecretList.Items))
	}

	pod := instance.GetPod(monCluster, adminKeyringSecretList.Items[0].GetName())
	pod.Namespace = s.daemon.GetNamespace()

	if err := controllerutil.SetControllerReference(s.daemon, pod, scheme); err != nil {
		return err
	}

	return c.Create(context.TODO(), pod)
}
