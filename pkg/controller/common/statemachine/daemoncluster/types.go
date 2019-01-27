package daemoncluster

import (
	"context"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TransitionFunc func(client.Client, *runtime.Scheme) error

type CephDaemonClusterStateMachine interface {
	State() cephv1alpha1.CephDaemonClusterState
	GetTransition(ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonClusterState)
}

type ReadOnlyClient interface {
	Get(context.Context, types.NamespacedName, runtime.Object) error
	List(context.Context, *client.ListOptions, runtime.Object) error
}

type DaemonCluster interface {
	metav1.Object
	Disabled() bool
	GetDaemonType() cephv1alpha1.CephDaemonType
	GetState() cephv1alpha1.CephDaemonClusterState
	GetCephClusterName() string
	GetImage() cephv1alpha1.ImageSpec
	DesiredReplicas() int
}
