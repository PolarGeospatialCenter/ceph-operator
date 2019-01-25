package cephdaemon

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type TransitionFunc func(client.Client, *runtime.Scheme) error
type podCheckFunc func(*corev1.Pod) bool

type CephDaemonStateMachine interface {
	State() cephv1alpha1.CephDaemonState
	GetTransition(ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonState)
}

type ReadOnlyClient interface {
	Get(context.Context, types.NamespacedName, runtime.Object) error
	List(context.Context, *client.ListOptions, runtime.Object) error
}

func NewCephDaemonStateMachine(daemon *cephv1alpha1.CephDaemon,
	daemonCluster *cephv1alpha1.CephDaemonCluster, logger logr.Logger) CephDaemonStateMachine {

	switch daemon.Spec.DaemonType {
	case cephv1alpha1.CephDaemonTypeMgr:
		return &MgrStateMachine{BaseStateMachine: newBaseStateMachine(daemon, daemonCluster, logger)}
	case cephv1alpha1.CephDaemonTypeMds:
		return &MdsStateMachine{BaseStateMachine: newBaseStateMachine(daemon, daemonCluster, logger)}
	default:
		return nil
	}
}

func newBaseStateMachine(daemon *cephv1alpha1.CephDaemon,
	daemonCluster *cephv1alpha1.CephDaemonCluster, logger logr.Logger) *BaseStateMachine {
	return &BaseStateMachine{daemon: daemon, daemonCluster: daemonCluster, logger: logger}
}

type BaseStateMachine struct {
	daemon        *cephv1alpha1.CephDaemon
	daemonCluster *cephv1alpha1.CephDaemonCluster
	logger        logr.Logger
}

func (s *BaseStateMachine) daemonEnabled() bool {
	return !s.daemon.Spec.Disabled && s.daemonCluster.GetState() != cephv1alpha1.CephDaemonClusterStateIdle
}

func (s *BaseStateMachine) State() cephv1alpha1.CephDaemonState {
	return s.daemon.GetState()
}

func (s *BaseStateMachine) checkPod(client ReadOnlyClient, checkFunc podCheckFunc) (bool, error) {

	pod := &corev1.Pod{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Name:      s.daemon.GetPodName(),
		Namespace: s.daemon.GetNamespace(),
	}, pod)
	if err != nil {
		return false, err
	}

	return checkFunc(pod), nil
}

func (s *BaseStateMachine) deletePod(client client.Client, scheme *runtime.Scheme) error {

	pod := &corev1.Pod{}
	pod.Name = s.daemon.GetPodName()
	pod.Namespace = s.daemon.GetNamespace()
	err := client.Delete(context.TODO(), pod)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (s *BaseStateMachine) logError(client client.Client, scheme *runtime.Scheme) error {
	s.logger.Info("ceph daemon is in error state")
	return nil
}

func (s *BaseStateMachine) launchPod(client client.Client, scheme *runtime.Scheme) error {
	daemonType := s.daemon.Spec.DaemonType

	pod := s.daemon.GetBasePod()

	envs := []corev1.EnvVar{corev1.EnvVar{
		Name:  "CMD",
		Value: fmt.Sprintf("start_%s", daemonType),
	},
		corev1.EnvVar{
			Name:  "DAEMON_ID",
			Value: s.daemon.Spec.ID,
		},
	}

	keyringName := fmt.Sprintf("ceph-%s-client.bootstrap-%s-keyring", s.daemon.Spec.ClusterName, daemonType)
	volumeMounts := []corev1.VolumeMount{corev1.VolumeMount{
		Name:      fmt.Sprintf("%s-bootstrap-keyring", daemonType),
		MountPath: fmt.Sprintf("/keyrings/client.bootstrap-%s", daemonType),
	}}

	volumes := []corev1.Volume{corev1.Volume{
		Name: fmt.Sprintf("%s-bootstrap-keyring", daemonType),
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: keyringName,
			},
		},
	}}

	pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, envs...)
	pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, volumeMounts...)
	pod.Spec.Volumes = append(pod.Spec.Volumes, volumes...)

	if err := controllerutil.SetControllerReference(s.daemon, pod, scheme); err != nil {
		return err
	}

	err := client.Create(context.TODO(), pod)
	if !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (s *BaseStateMachine) GetTransition(client ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonState) {

	if !s.daemonEnabled() && s.State() != cephv1alpha1.CephDaemonStateCleanup && s.State() != cephv1alpha1.CephDaemonStateIdle {
		return nil, cephv1alpha1.CephDaemonStateCleanup
	}

	switch s.State() {
	case cephv1alpha1.CephDaemonStateIdle:
		if s.daemonEnabled() {
			return nil, cephv1alpha1.CephDaemonStateLaunching
		}

	case cephv1alpha1.CephDaemonStateLaunching:
		return s.launchPod, cephv1alpha1.CephDaemonStateWaitForRun

	case cephv1alpha1.CephDaemonStateWaitForRun:
		running, err := s.checkPod(client, podRunning)
		if err != nil {
			return nil, cephv1alpha1.CephDaemonStateError
		}
		if running {
			return nil, cephv1alpha1.CephDaemonStateWaitForReady
		}

	case cephv1alpha1.CephDaemonStateWaitForReady:
		ready, err := s.checkPod(client, podReady)
		if err != nil {
			return nil, cephv1alpha1.CephDaemonStateError
		}
		if ready {
			return nil, cephv1alpha1.CephDaemonStateReady
		}

	case cephv1alpha1.CephDaemonStateReady:
		if _, err := s.checkPod(client, func(*corev1.Pod) bool { return true }); err != nil {
			return nil, cephv1alpha1.CephDaemonStateError
		}

	case cephv1alpha1.CephDaemonStateError:
		return s.logError, cephv1alpha1.CephDaemonStateCleanup

	case cephv1alpha1.CephDaemonStateCleanup:
		return s.deletePod, cephv1alpha1.CephDaemonStateIdle

	default:
		return nil, cephv1alpha1.CephDaemonStateCleanup
	}

	return nil, s.State()
}

type MgrStateMachine struct {
	*BaseStateMachine
}

func (s *MgrStateMachine) GetTransition(client ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonState) {

	switch s.State() {
	default:
		return s.BaseStateMachine.GetTransition(client)
	}
}

type MdsStateMachine struct {
	*BaseStateMachine
}

func (s *MdsStateMachine) GetTransition(client ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonState) {

	switch s.State() {
	default:
		return s.BaseStateMachine.GetTransition(client)
	}
}

func podRunning(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning
}

func podReady(pod *corev1.Pod) bool {
	for _, status := range pod.Status.Conditions {
		if status.Type == corev1.PodReady {
			return status.Status == corev1.ConditionTrue
		}
	}
	return false
}
