package cephdaemoncluster

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
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

	daemonList, err := r.listDaemons(instance.GetDaemonType(), instance.Spec.ClusterName)
	if err != nil {
		return reconcile.Result{}, err
	}

	daemonCount := len(daemonList.Items)

	if daemonCount > instance.Spec.Replicas {
		return reconcile.Result{}, r.deleteDaemon(instance)
	}

	if daemonCount < instance.Spec.Replicas {
		return reconcile.Result{}, r.createDaemon(instance)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileCephDaemonCluster) createDaemon(c *cephv1alpha1.CephDaemonCluster) error {
	daemon := cephv1alpha1.NewCephDaemon(c.GetDaemonType(), c.GetCephClusterName())

	daemon.Spec.Image = c.GetImage()
	daemon.Spec.CephConfConfigMapName = c.GetCephConfConfigMapName()
	daemon.Namespace = c.GetNamespace()

	if err := controllerutil.SetControllerReference(c, daemon, r.scheme); err != nil {
		return err
	}

	err := r.client.Create(context.TODO(), daemon)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (r *ReconcileCephDaemonCluster) deleteDaemon(c *cephv1alpha1.CephDaemonCluster) error {
	daemons, err := r.listDaemons(c.GetDaemonType(), c.Spec.ClusterName)
	if err != nil {
		return err
	}

	if len(daemons.Items) < 1 {
		return fmt.Errorf("unabled to delete daemon, no daemons found")
	}

	var toDelete cephv1alpha1.CephDaemon

	rand.Seed(time.Now().UnixNano())
	toDelete = daemons.Items[rand.Int()%len(daemons.Items)]

	return r.client.Delete(context.TODO(), &toDelete)
}

func (r *ReconcileCephDaemonCluster) listDaemons(daemonType cephv1alpha1.CephDaemonType, clusterName string) (*cephv1alpha1.CephDaemonList, error) {
	daemonList := &cephv1alpha1.CephDaemonList{}
	daemonListOptions := &client.ListOptions{}
	daemonListOptions.MatchingLabels(map[string]string{
		cephv1alpha1.ClusterNameLabel: clusterName,
		cephv1alpha1.DaemonTypeLabel:  string(daemonType),
	})

	return daemonList, r.client.List(context.TODO(), daemonListOptions, daemonList)
}
