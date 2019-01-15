package cephmoncluster

import (
	"context"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"

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

	fullMonMap, err := r.getMonMap(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	monMap := fullMonMap.GetInitalMonMap()

	switch instance.GetMonClusterState() {

	case cephv1alpha1.MonClusterIdle:
		if fullMonMap.Empty() {
			return reconcile.Result{}, nil
		}

		instance.SetMonClusterState(cephv1alpha1.MonClusterLaunching)
		instance.Status.StartEpoch++

		cm, err := instance.GetMonMapConfigMap(monMap)
		cm.Namespace = instance.Namespace
		if err != nil {
			return reconcile.Result{}, err
		}
		_, err = r.createOrUpdate(cm)
		if err != nil {
			return reconcile.Result{}, err
		}

		_, err = r.updateAndRequeue(instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		if fullMonMap.CountInitalMembers() > 0 {
			return reconcile.Result{}, nil
		}

		initialMonNamespacedName := fullMonMap.GetRandomEntry().NamespacedName
		initialMon := &cephv1alpha1.CephMon{}

		err = r.client.Get(context.TODO(), initialMonNamespacedName, initialMon)
		if err != nil {
			return reconcile.Result{}, err
		}

		initialMon.Status.InitalMember = true
		return r.updateAndRequeue(initialMon)

	case cephv1alpha1.MonClusterLaunching:
		if monMap.QuorumAtEpoch(instance.Status.StartEpoch) {
			cm, err := instance.GetMonMapConfigMap(monMap)
			cm.Namespace = instance.Namespace
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

		totalInQuorum := monMap.CountInState(cephv1alpha1.MonInQuorum)
		if totalInQuorum >= monMap.QuorumCount() {
			instance.SetMonClusterState(cephv1alpha1.MonClusterInQuorum)
			return r.updateAndRequeue(instance)
		}

		return reconcile.Result{}, nil

	case cephv1alpha1.MonClusterInQuorum:

		totalInQuorum := monMap.CountInState(cephv1alpha1.MonInQuorum)
		if totalInQuorum < monMap.QuorumCount() {
			instance.SetMonClusterState(cephv1alpha1.MonClusterLostQuorum)
			return r.updateAndRequeue(instance)
		}

		return reconcile.Result{}, nil

	case cephv1alpha1.MonClusterLostQuorum:

		if monMap.AllInState(cephv1alpha1.MonIdle) {
			instance.SetMonClusterState(cephv1alpha1.MonClusterIdle)
			return r.updateAndRequeue(instance)
		}

		return reconcile.Result{}, nil

	default:
		instance.SetMonClusterState(cephv1alpha1.MonClusterLostQuorum)
		return r.updateAndRequeue(instance)
	}
}

func (r *ReconcileCephMonCluster) getMonMap(instance *cephv1alpha1.CephMonCluster) (cephv1alpha1.MonMap, error) {
	monitors := &cephv1alpha1.CephMonList{}
	listOptions := &client.ListOptions{}
	listOptions.MatchingLabels(map[string]string{cephv1alpha1.MonitorClusterLabel: instance.Spec.ClusterName})

	err := r.client.List(context.TODO(), listOptions, monitors)
	if err != nil {
		return nil, err
	}

	monMap := make(cephv1alpha1.MonMap, len(monitors.Items))

	for _, mon := range monitors.Items {
		monMap[mon.Spec.ID] = mon.GetMonMapEntry()
	}

	return monMap, nil
}

func (r *ReconcileCephMonCluster) updateAndRequeue(object runtime.Object) (reconcile.Result, error) {
	err := r.client.Update(context.TODO(), object)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileCephMonCluster) createOrUpdate(object runtime.Object) (reconcile.Result, error) {
	err := r.client.Create(context.TODO(), object)
	if !errors.IsAlreadyExists(err) {
		return reconcile.Result{}, err
	}

	err = r.client.Update(context.TODO(), object)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
