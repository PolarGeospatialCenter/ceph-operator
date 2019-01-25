package cephcluster

import (
	"context"
	"fmt"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	err = r.updateCephConfConfigMap(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Create or update monitor Service
	svc := instance.GetMonitorService()
	svc.Namespace = instance.Namespace

	err = r.createIfNotFound(svc)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Create Daemon Clusters
	for _, o := range []DaemonClusterObject{
		&cephv1alpha1.CephMonCluster{},
		cephv1alpha1.NewCephDaemonCluster(cephv1alpha1.CephDaemonTypeMgr),
		cephv1alpha1.NewCephDaemonCluster(cephv1alpha1.CephDaemonTypeMds),
	} {
		err = r.createDaemonCluster(o, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return r.update(instance)
}

type DaemonClusterObject interface {
	metav1.Object
	runtime.Object
	SetCephClusterName(string)
	SetImage(cephv1alpha1.ImageSpec)
	SetCephConfConfigMapName(string)
	GetDaemonType() cephv1alpha1.CephDaemonType
}

func (r *ReconcileCephCluster) createDaemonCluster(o DaemonClusterObject, cluster *cephv1alpha1.CephCluster) error {
	o.SetName(cluster.GetName())
	o.SetNamespace(cluster.GetNamespace())
	o.SetCephClusterName(cluster.GetName())
	o.SetCephConfConfigMapName(cluster.GetCephConfigMapName())
	o.SetLabels(map[string]string{
		cephv1alpha1.ClusterNameLabel: cluster.GetName(),
		cephv1alpha1.DaemonTypeLabel:  o.GetDaemonType().String(),
	})

	switch v := o.(type) {
	case *cephv1alpha1.CephMonCluster:
		o.SetImage(cluster.Spec.MonImage)
	case *cephv1alpha1.CephDaemonCluster:
		o.SetName(fmt.Sprintf("%s-%s", cluster.GetName(), v.Spec.DaemonType))
		v.Spec.Replicas = 3
		switch v.Spec.DaemonType {
		case cephv1alpha1.CephDaemonTypeMgr:
			o.SetImage(cluster.Spec.MgrImage)
		case cephv1alpha1.CephDaemonTypeMds:
			o.SetImage(cluster.Spec.MdsImage)
		default:
			return fmt.Errorf("Could not determine image for type %s", v.Spec.DaemonType)
		}

	default:
		return fmt.Errorf("Could not determine image for type %T", o)
	}

	if err := controllerutil.SetControllerReference(cluster, o, r.scheme); err != nil {
		return err
	}

	return r.createIfNotFound(o)
}

func (r *ReconcileCephCluster) createIfNotFound(o runtime.Object) error {
	err := r.client.Create(context.TODO(), o)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (r *ReconcileCephCluster) update(object runtime.Object) (reconcile.Result, error) {
	err := r.client.Update(context.TODO(), object)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileCephCluster) updateCephConfConfigMap(instance *cephv1alpha1.CephCluster) error {
	configMap, err := instance.GetCephConfigMap()
	if err != nil {
		return err
	}
	configMap.Namespace = instance.GetNamespace()

	return r.createIfNotFound(configMap)
}
