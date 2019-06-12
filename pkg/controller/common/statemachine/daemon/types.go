package daemon

import (
	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common/statemachine"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CephDaemonStateMachine interface {
	State() cephv1alpha1.CephDaemonState
	GetTransition(statemachine.ReadOnlyClient) (statemachine.TransitionFunc, cephv1alpha1.CephDaemonState)
}

type Daemon interface {
	metav1.Object
	GetPodName() string
	GetDisabled() bool
	GetDaemonID() string
	GetDaemonType() cephv1alpha1.CephDaemonType
	GetState() cephv1alpha1.CephDaemonState
	GetCephClusterName() string
	GetImage() cephv1alpha1.ImageSpec
	GetBasePod() *corev1.Pod
	GetVolumeClaimTemplate() (*corev1.PersistentVolumeClaim, error)
	StateIn(...cephv1alpha1.CephDaemonState) bool
}
