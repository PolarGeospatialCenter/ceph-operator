package cephmoncluster

import (
	"context"
	"time"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
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

var log = logf.Log.WithName("controller_cephmoncluster")

type monitorList []cephv1alpha1.CephMon

func (m monitorList) allInState(state cephv1alpha1.MonState) bool {
	for _, mon := range m {
		if mon.Status.State != state {
			return false
		}
	}
	return true
}

func (m monitorList) countInState(state cephv1alpha1.MonState) int {
	var count int
	for _, mon := range m {
		if mon.Status.State == state {
			count++
		}
	}
	return count
}

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
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cephv1alpha1.CephMonCluster{},
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

	switch instance.GetMonClusterState() {

	case cephv1alpha1.MonClusterIdle:
		if instance.Status.MonMapEmpty() {
			return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, nil
		}

		instance.SetMonClusterState(cephv1alpha1.MonClusterLaunching)
		instance.Status.StartEpoch++
		return r.updateAndRequeue(instance)

	case cephv1alpha1.MonClusterLaunching:
		if instance.Status.MonMapQuorumAtEpoch(instance.Status.StartEpoch) {
			cm, err := instance.GetMonMapConfigMap()
			if err != nil {
				return reconcile.Result{}, err
			}
			_, err = r.createOrUpdate(cm)
			if err != nil {
				return reconcile.Result{}, err
			}

			instance.SetMonClusterState(cephv1alpha1.MonClusterEstablishingQuorum)
			return r.updateAndRequeue(instance)
		}
		return reconcile.Result{}, nil

	case cephv1alpha1.MonClusterEstablishingQuorum:
		monitors, err := r.getMonitorsInMonMap(instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		totalInQuorum := monitors.countInState(cephv1alpha1.MonInQuorum)
		if totalInQuorum >= instance.Status.MonMapQuorumCount() {
			instance.SetMonClusterState(cephv1alpha1.MonClusterInQuorum)
			return r.updateAndRequeue(instance)
		}

		return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, nil

	case cephv1alpha1.MonClusterInQuorum:
		monitors, err := r.getMonitorsInMonMap(instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		totalInQuorum := monitors.countInState(cephv1alpha1.MonInQuorum)
		if totalInQuorum < instance.Status.MonMapQuorumCount() {
			instance.SetMonClusterState(cephv1alpha1.MonClusterLostQuorum)
			return r.updateAndRequeue(instance)
		}

		return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil

	case cephv1alpha1.MonClusterLostQuorum:
		monitors, err := r.getMonitorsInMonMap(instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		if monitors.allInState(cephv1alpha1.MonIdle) {
			instance.SetMonClusterState(cephv1alpha1.MonClusterIdle)
			return r.updateAndRequeue(instance)
		}

		return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, nil

	default:
		instance.SetMonClusterState(cephv1alpha1.MonClusterLostQuorum)
		return r.updateAndRequeue(instance)
	}
}

func (r *ReconcileCephMonCluster) getMonitorsInMonMap(instance *cephv1alpha1.CephMonCluster) (monitorList, error) {
	monitors := &cephv1alpha1.CephMonList{}
	err := r.client.List(context.TODO(), &client.ListOptions{}, monitors)
	if err != nil {
		return nil, err
	}

	matchingMonitors := make(monitorList, 0, len(monitors.Items))
	for _, mon := range monitors.Items {
		if instance.Status.MonMapContains(&mon) {
			matchingMonitors = append(matchingMonitors, mon)
		}
	}

	return matchingMonitors, nil
}

func (r *ReconcileCephMonCluster) updateAndRequeue(object runtime.Object) (reconcile.Result, error) {
	err := r.client.Update(context.TODO(), object)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{Requeue: true}, nil
}

func (r *ReconcileCephMonCluster) createOrUpdate(object runtime.Object) (reconcile.Result, error) {
	err := r.client.Create(context.TODO(), object)
	if err != nil && !errors.IsAlreadyExists(err) {
		return reconcile.Result{}, err
	}

	err = r.client.Update(context.TODO(), object)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
