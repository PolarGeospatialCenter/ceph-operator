package cephmon

import (
	"context"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_cephmon")

// Add creates a new CephMon Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCephMon{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cephmon-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CephMon
	err = c.Watch(&source.Kind{Type: &cephv1alpha1.CephMon{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner CephMon
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cephv1alpha1.CephMon{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileCephMon{}

// ReconcileCephMon reconciles a CephMon object
type ReconcileCephMon struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a CephMon object and makes changes based on the state read
// and what is in the CephMon.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephMon) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CephMon")

	// Fetch the CephMon instance
	instance := &cephv1alpha1.CephMon{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Return if disabled
	if instance.GetDisabled() {
		return reconcile.Result{}, nil
	}

	// Lookup cluster
	cluster := &cephv1alpha1.CephCluster{}
	clusterNamespacedName := &types.NamespacedName{
		Namespace: request.Namespace,
		Name:      instance.Spec.ClusterName,
	}
	err = r.client.Get(context.TODO(), *clusterNamespacedName, cluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Create PVC
	pvc := instance.GetVolumeClaimTemplate()
	pvc.Namespace = request.Namespace
	common.UpdateOwnerReferences(instance, pvc)

	err = r.client.Create(context.TODO(), pvc)

	if err != nil && !errors.IsAlreadyExists(err) {
		return reconcile.Result{}, err
	}

	// Create Pod
	pod := instance.GetPod(cluster.GetMonImage(), "ceph-conf")
	pod.Namespace = request.Namespace
	common.UpdateOwnerReferences(instance, pod)

	err = r.client.Create(context.TODO(), pod)
	if err != nil && !errors.IsAlreadyExists(err) {
		return reconcile.Result{}, err
	}

	// Attach ENV Vars to Pod
	// Cmd
	// Cluster
	// MonIP
	// MonName
	// k8snamespace
	// Attach ceph.conf Configmap to Pod
	// Attach Labels for service
	// Attach bootstrap keyrings?

	return reconcile.Result{}, nil

}
