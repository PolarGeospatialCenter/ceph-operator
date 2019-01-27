package cephdaemoncluster

import (
	"context"
	"fmt"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common"
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

	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common/statemachine/daemoncluster"
)

var log = logf.Log.WithName("controller_cephdaemoncluster")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new CephDaemonCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCephDaemonCluster{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cephdaemoncluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CephDaemonCluster
	err = c.Watch(&source.Kind{Type: &cephv1alpha1.CephDaemonCluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &cephv1alpha1.CephDaemon{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cephv1alpha1.CephDaemonCluster{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &cephv1alpha1.CephCluster{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &common.CephClusterEventMapper{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ApiVersion: cephv1alpha1.SchemeGroupVersion.String(), Kind: "CephDaemonCluster"},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileCephDaemonCluster{}

// ReconcileCephDaemonCluster reconciles a CephDaemonCluster object
type ReconcileCephDaemonCluster struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a CephDaemonCluster object and makes changes based on the state read
// and what is in the CephDaemonCluster.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephDaemonCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CephDaemonCluster")

	// Fetch the CephDaemonCluster instance
	instance := &cephv1alpha1.CephDaemonCluster{}
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

	// Label ourselves with our TYPE and ClusterName
	labelsUpdated := updateLabels(instance)
	if labelsUpdated {
		return reconcile.Result{}, r.updateObject(instance)
	}

	// Lookup CephCluster - // Only Return One
	cephCluster, err := r.getCephCluster(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	dsm := daemoncluster.NewStateMachine(instance, cephCluster, reqLogger)

	currentState := dsm.State()
	transtionFunc, nextState := dsm.GetTransition(r.client)

	if transtionFunc != nil {
		err = transtionFunc(r.client, r.scheme)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	if nextState == currentState {
		return reconcile.Result{}, nil
	}

	reqLogger.Info(fmt.Sprintf("transitioning from %s to %s", currentState, nextState))
	instance.SetState(nextState)
	return reconcile.Result{}, r.updateObject(instance)
}

func updateLabels(d *cephv1alpha1.CephDaemonCluster) bool {
	var updated bool
	labels := d.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	if val, ok := labels[cephv1alpha1.ClusterNameLabel]; !ok || val != d.Spec.ClusterName {
		labels[cephv1alpha1.ClusterNameLabel] = d.Spec.ClusterName
		d.SetLabels(labels)
		updated = true
	}
	if val, ok := labels[cephv1alpha1.DaemonTypeLabel]; !ok || val != d.Spec.DaemonType.String() {
		labels[cephv1alpha1.DaemonTypeLabel] = d.Spec.DaemonType.String()
		d.SetLabels(labels)
		updated = true
	}

	return updated
}

func (r *ReconcileCephDaemonCluster) updateObject(object runtime.Object) error {
	return r.client.Update(context.TODO(), object)
}

func (r *ReconcileCephDaemonCluster) getCephCluster(d *cephv1alpha1.CephDaemonCluster) (*cephv1alpha1.CephCluster, error) {
	cephCluster := &cephv1alpha1.CephCluster{}
	cephClusterNamespacedName := types.NamespacedName{
		Name:      d.GetCephClusterName(),
		Namespace: d.GetNamespace(),
	}

	return cephCluster, r.client.Get(context.TODO(), cephClusterNamespacedName, cephCluster)
}
