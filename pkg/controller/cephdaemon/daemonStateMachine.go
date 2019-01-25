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

const (
	StateIdle         cephv1alpha1.CephDaemonState = "Idle"
	StateLaunching                                 = "Launching"
	StateWaitForRun                                = "Wait for Run"
	StateWaitForReady                              = "Wait for Ready"
	StateReady                                     = "Ready"
	StateError                                     = "Error"
	StateCleanup                                   = "Cleanup"
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

	return &MgrStateMachine{BaseStateMachine: newBaseStateMachine(daemon, daemonCluster, logger)}
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
	return !s.daemon.Spec.Disabled
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

func (s *BaseStateMachine) GetTransition(client ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonState) {

	switch s.State() {
	case StateIdle:
		if s.daemonEnabled() {
			return nil, StateLaunching
		}
	case StateWaitForRun:
		running, err := s.checkPod(client, podRunning)
		if err != nil {
			return nil, StateError
		}
		if running {
			return nil, StateWaitForReady
		}
	case StateWaitForReady:
		ready, err := s.checkPod(client, podReady)
		if err != nil {
			return nil, StateError
		}
		if ready {
			return nil, StateReady
		}

	case StateReady:
		if !s.daemonEnabled() {
			return nil, StateCleanup
		}
		if _, err := s.checkPod(client, func(*corev1.Pod) bool { return true }); err != nil {
			return nil, StateError
		}

	case StateError:
		return s.logError, StateCleanup

	case StateCleanup:
		return s.deletePod, StateIdle

	default:
		return nil, StateCleanup
	}

	return nil, s.State()
}

type MgrStateMachine struct {
	*BaseStateMachine
}

func (s *MgrStateMachine) GetTransition(client ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonState) {

	switch s.State() {
	case StateLaunching:
		if s.daemonEnabled() {
			return s.launchPod, StateWaitForRun
		}

	default:
		return s.BaseStateMachine.GetTransition(client)
	}

	return nil, s.State()
}

func (s *MgrStateMachine) launchPod(client client.Client, scheme *runtime.Scheme) error {
	daemonType := s.daemon.Spec.DaemonType

	pod := s.daemon.GetBasePod()

	envs := []corev1.EnvVar{corev1.EnvVar{
		Name:  "CMD",
		Value: fmt.Sprintf("start_%s", daemonType),
	},
		corev1.EnvVar{
			Name:  "MGR_ID",
			Value: s.daemon.Spec.ID,
		},
	}

	keyringName := fmt.Sprintf("ceph-%s-client.bootstrap-%s-keyring", s.daemon.Spec.ClusterName, daemonType)
	volumeMounts := []corev1.VolumeMount{corev1.VolumeMount{
		Name:      "mgr-bootstrap-keyring",
		MountPath: fmt.Sprintf("/keyrings/client.bootstrap-%s", daemonType),
	}}

	volumes := []corev1.Volume{corev1.Volume{
		Name: "mgr-bootstrap-keyring",
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
