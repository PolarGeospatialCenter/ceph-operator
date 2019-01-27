package cephmoncluster

import (
	"context"
	"fmt"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common"

	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common/statemachine/daemoncluster"

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

var log = logf.Log.WithName("controller_cephmoncluster")

// Add creates a new CephMonCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCephMonCluster{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cephmoncluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CephMonCluster
	err = c.Watch(&source.Kind{Type: &cephv1alpha1.CephMonCluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner CephMonCluster
	err = c.Watch(&source.Kind{Type: &cephv1alpha1.CephMon{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &MonEventMapper{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &cephv1alpha1.CephCluster{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &common.CephClusterEventMapper{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ApiVersion: cephv1alpha1.SchemeGroupVersion.String(), Kind: "CephMonCluster"},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileCephMonCluster{}

// ReconcileCephMonCluster reconciles a CephMonCluster object
type ReconcileCephMonCluster struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a CephMonCluster object and makes changes based on the state read
// and what is in the CephMonCluster.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephMonCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CephMonCluster")

	// Fetch the CephMonCluster instance
	instance := &cephv1alpha1.CephMonCluster{}
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

func (r *ReconcileCephMonCluster) updateObject(object runtime.Object) error {
	return r.client.Update(context.TODO(), object)
}

func (r *ReconcileCephMonCluster) getCephCluster(d *cephv1alpha1.CephMonCluster) (*cephv1alpha1.CephCluster, error) {
	cephCluster := &cephv1alpha1.CephCluster{}
	cephClusterNamespacedName := types.NamespacedName{
		Name:      d.GetCephClusterName(),
		Namespace: d.GetNamespace(),
	}

	return cephCluster, r.client.Get(context.TODO(), cephClusterNamespacedName, cephCluster)
}
