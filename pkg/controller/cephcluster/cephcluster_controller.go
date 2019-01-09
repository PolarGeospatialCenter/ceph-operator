package cephcluster

import (
	"context"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
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

var log = logf.Log.WithName("controller_cephcluster")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new CephCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCephCluster{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cephcluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CephCluster
	err = c.Watch(&source.Kind{Type: &cephv1alpha1.CephCluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner CephCluster
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cephv1alpha1.CephCluster{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileCephCluster{}

// ReconcileCephCluster reconciles a CephCluster object
type ReconcileCephCluster struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CephCluster")

	// Fetch the CephCluster instance
	instance := &cephv1alpha1.CephCluster{}
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

	// Create Configmap
	// Merge defaults with the config stored in cluster object

	configMap, err := instance.GetCephConfigMap()
	if err != nil {
		return reconcile.Result{}, err
	}
	configMap.Namespace = request.Namespace

	// Create or update configmap.

	err = r.client.Create(context.TODO(), configMap)

	if err != nil && !errors.IsAlreadyExists(err) {
		return reconcile.Result{}, err
	}

	if errors.IsAlreadyExists(err) {
		err = r.client.Update(context.TODO(), configMap)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Create or update Monitor Service
	svc := instance.GetMonitorService()
	svc.Namespace = instance.Namespace

	err = r.client.Create(context.TODO(), svc)
	if err != nil && !errors.IsAlreadyExists(err) {
		return reconcile.Result{}, err
	}

	if errors.IsAlreadyExists(err) {
		err = r.client.Update(context.TODO(), configMap)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Generate Monitor Keyring
	monKeyring := MON_KEYRING
	monSecret := &corev1.Secret{}
	monSecretNamespacedName := &types.NamespacedName{
		Namespace: request.Namespace,
		Name:      monKeyring.GetSecretName(instance.GetName()),
	}
	err = r.client.Get(context.TODO(), *monSecretNamespacedName, monSecret)
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	if errors.IsNotFound(err) {
		err = monKeyring.GenerateKey()
		if err != nil {
			return reconcile.Result{}, err
		}

		monSecret = monKeyring.GetSecret(instance.GetName())
		monSecret.Namespace = request.Namespace
		err = r.client.Create(context.TODO(), monSecret)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Generate Admin Keyring
	adminKeyring := CLIENT_ADMIN_KEYRING
	adminSecret := &corev1.Secret{}
	adminSecretNamespacedName := &types.NamespacedName{
		Namespace: request.Namespace,
		Name:      adminKeyring.GetSecretName(instance.GetName()),
	}
	err = r.client.Get(context.TODO(), *adminSecretNamespacedName, adminSecret)
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	if errors.IsNotFound(err) {
		err = adminKeyring.GenerateKey()
		if err != nil {
			return reconcile.Result{}, err
		}

		adminSecret = adminKeyring.GetSecret(instance.GetName())
		adminSecret.Namespace = request.Namespace
		err = r.client.Create(context.TODO(), adminSecret)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Launch MGR

	// Launch MDS

	// Launch RGW?

	return reconcile.Result{}, nil

}
