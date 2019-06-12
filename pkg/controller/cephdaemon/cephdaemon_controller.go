package cephdaemon

import (
	"context"
	"fmt"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common/statemachine/daemon"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_cephdaemon")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new CephDaemon Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCephDaemon{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cephdaemon-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CephDaemon
	err = c.Watch(&source.Kind{Type: &cephv1alpha1.CephDaemon{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &cephv1alpha1.CephDaemonCluster{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &DaemonClusterEventMapper{client: mgr.GetClient(), scheme: mgr.GetScheme()},
	})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner CephDaemon
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cephv1alpha1.CephDaemon{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileCephDaemon{}

// ReconcileCephDaemon reconciles a CephDaemon object
type ReconcileCephDaemon struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a CephDaemon object and makes changes based on the state read
// and what is in the CephDaemon.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephDaemon) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CephDaemon")

	// Fetch the CephDaemon instance
	instance := &cephv1alpha1.CephDaemon{}
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

	// Set CephDaemon instance as the owner and controller
	// if err := controllerutil.SetControllerReference(instance, pod, r.scheme); err != nil {
	// 	return reconcile.Result{}, err
	// }

	// Label ourselves with our TYPE and ClusterName
	labelsUpdated := updateLabels(instance)
	if labelsUpdated {
		return reconcile.Result{}, r.updateObject(instance)
	}

	// Lookup Daemon Cluster - // Only Return One
	daemonCluster, err := r.getDaemonCluster(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	dsm := daemon.NewStateMachine(instance, daemonCluster, reqLogger)

	currentState := dsm.State()
	transtionFunc, nextState := dsm.GetTransition(r.client)

	if nextState == currentState {
		return reconcile.Result{}, nil
	}

	if transtionFunc != nil {
		err = transtionFunc(r.client, r.scheme)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger.Info(fmt.Sprintf("transitioning from %s to %s", currentState, nextState))
	instance.SetState(nextState)
	return reconcile.Result{}, r.updateObject(instance)

	// Get Current State
	// Get Next state, and call function for transtion
	// If success, update current state

}

func updateLabels(d *cephv1alpha1.CephDaemon) bool {
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

func (r *ReconcileCephDaemon) updateObject(object runtime.Object) error {
	return r.client.Update(context.TODO(), object)
}

func (r *ReconcileCephDaemon) getDaemonCluster(d *cephv1alpha1.CephDaemon) (*cephv1alpha1.CephDaemonCluster, error) {
	daemonClusterList := &cephv1alpha1.CephDaemonClusterList{}
	daemonClusterListOptions := &client.ListOptions{}
	daemonClusterListOptions.MatchingLabels(map[string]string{
		cephv1alpha1.ClusterNameLabel: d.Spec.ClusterName,
		cephv1alpha1.DaemonTypeLabel:  d.Spec.DaemonType.String(),
	})
	err := r.client.List(context.TODO(), daemonClusterListOptions, daemonClusterList)
	if err != nil {
		return nil, err
	}

	daemonClusterCount := len(daemonClusterList.Items)
	if daemonClusterCount > 1 {
		return nil, fmt.Errorf("found %d %s daemon clusters, should only have 1", daemonClusterCount, d.Spec.DaemonType)
	}

	if daemonClusterCount == 0 {
		log.Info("No %s daemon cluster found. Ignoring until one exists...", d.Spec.DaemonType)
		return nil, nil
	}

	return &daemonClusterList.Items[0], nil
}
