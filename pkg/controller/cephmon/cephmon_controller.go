package cephmon

import (
	"context"
	"net"
	"time"

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

	// Lookup monCluster
	monCluster := &cephv1alpha1.CephMonCluster{}
	monClusterNamespacedName := &types.NamespacedName{
		Namespace: request.Namespace,
		Name:      cluster.Status.MonClusterName,
	}
	err = r.client.Get(context.TODO(), *monClusterNamespacedName, monCluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Check for disabled or lost quorum states
	if (instance.GetDisabled() || monCluster.CheckMonClusterState(cephv1alpha1.MonClusterLostQuorum)) &&
		!instance.CheckMonState(cephv1alpha1.MonCleanup, cephv1alpha1.MonIdle) {

		instance.Status.State = cephv1alpha1.MonCleanup
		_, err = r.updateAndRequeue(instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	switch instance.GetMonState() {
	case cephv1alpha1.MonError:
		log.Info("Monitor is in error state, cleaning up", "MonitorID", instance.Spec.ID)
		instance.Status.State = cephv1alpha1.MonCleanup
		return r.updateAndRequeue(instance)

	case cephv1alpha1.MonCleanup:
		pod := &corev1.Pod{}
		pod.Namespace = request.Namespace
		pod.Name = instance.GetPodName()
		err = r.client.Delete(context.TODO(), pod)
		if err != nil && !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}

		instance.Status.State = cephv1alpha1.MonIdle
		return r.updateAndRequeue(instance)

	case cephv1alpha1.MonIdle:
		switch monCluster.GetMonClusterState() {
		case cephv1alpha1.MonClusterInQuorum:
			instance.Status.State = cephv1alpha1.MonLaunchPod

		case cephv1alpha1.MonClusterLaunching:
			if monCluster.Status.MonMapContains(instance) {
				instance.Status.State = cephv1alpha1.MonLaunchPod
			}

		case cephv1alpha1.MonClusterIdle:
			// Update Monmap
			if monCluster.Status.MonMapEmpty() {
				err = r.updateMonMap(monCluster, instance.Spec.ID, cephv1alpha1.MonMapEntry{
					Port: 6789,
				})
				if errors.IsConflict(err) {
					// Another monitor beat us to the update, retry
					return reconcile.Result{Requeue: true}, nil
				} else if err != nil {
					return reconcile.Result{}, err
				}
			}
		default:
			return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, nil
		}

		err = r.client.Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, nil

	case cephv1alpha1.MonLaunchPod:
		if !monCluster.CheckMonClusterState(cephv1alpha1.MonClusterInQuorum,
			cephv1alpha1.MonClusterEstablishingQuorum,
			cephv1alpha1.MonClusterLaunching) {
			log.Info("Refusing to launch monitor while cluster is unexpected state",
				"ClusterState", monCluster.GetMonClusterState(), "MonitorId", instance.Spec.ID)
			return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, nil
		}
		// Create PVC
		pvc, err := instance.GetVolumeClaimTemplate()
		if err != nil {
			return reconcile.Result{}, err
		}
		pvc.Namespace = request.Namespace
		common.UpdateOwnerReferences(instance, pvc)

		err = r.client.Create(context.TODO(), pvc)

		if err != nil && !errors.IsAlreadyExists(err) {
			return reconcile.Result{}, err
		}

		monitorDiscoveryServiceName := cluster.GetMonitorDiscoveryService().GetName()

		// Create Pod
		pod := instance.GetPod(cluster.GetMonImage(), cluster.GetCephConfigMapName(),
			monitorDiscoveryServiceName, request.Namespace, cluster.Spec.ClusterDomain)
		pod.Namespace = request.Namespace
		common.UpdateOwnerReferences(instance, pod)

		err = r.client.Create(context.TODO(), pod)
		if err != nil && !errors.IsAlreadyExists(err) {
			return reconcile.Result{}, err
		}

		switch monCluster.GetMonClusterState() {
		case cephv1alpha1.MonClusterInQuorum:
			instance.Status.State = cephv1alpha1.MonWaitForPodReady
		case cephv1alpha1.MonClusterLaunching:
			instance.Status.State = cephv1alpha1.MonWaitForPodRun
		case cephv1alpha1.MonClusterEstablishingQuorum:
			instance.Status.State = cephv1alpha1.MonWaitForPodRun
		}
		return r.updateAndRequeue(instance)

	case cephv1alpha1.MonWaitForPodRun:
		updated, err := r.updateMonMapOnPodStatus(monCluster, instance, podRunning)
		if !updated || err != nil {
			return reconcile.Result{}, err
		}
		instance.Status.State = cephv1alpha1.MonWaitForPodReady
		return r.updateAndRequeue(instance)

	case cephv1alpha1.MonWaitForPodReady:
		updated, err := r.updateMonMapOnPodStatus(monCluster, instance, podInQuorum)
		if !updated || err != nil {
			return reconcile.Result{}, err
		}
		instance.Status.State = cephv1alpha1.MonInQuorum
		return r.updateAndRequeue(instance)
	case cephv1alpha1.MonInQuorum:
		quorum, _, err := r.checkPod(instance.GetPodName(), instance.Namespace, podInQuorum)
		if quorum || err != nil {
			return reconcile.Result{}, err
		}
		// out of quorum with no error
		instance.Status.State = cephv1alpha1.MonCleanup
		return r.updateAndRequeue(instance)
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

type podCheckFunc func(*corev1.Pod) bool

func (r *ReconcileCephMon) checkPod(podName, namespace string, checkFunc podCheckFunc) (bool, net.IP, error) {
	pod := &corev1.Pod{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Name:      podName,
		Namespace: namespace,
	}, pod)
	if errors.IsNotFound(err) {
		return false, net.IP{}, nil
	} else if err != nil {
		return false, net.IP{}, err
	}
	podIP := net.ParseIP(pod.Status.PodIP)

	return checkFunc(pod), podIP, nil
}

func podInQuorum(pod *corev1.Pod) bool {
	for _, status := range pod.Status.Conditions {
		if status.Type == cephv1alpha1.MonQuorumPodCondition {
			return status.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podRunning(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning
}

func (r *ReconcileCephMon) updateAndRequeue(object runtime.Object) (reconcile.Result, error) {
	err := r.client.Update(context.TODO(), object)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{Requeue: true}, nil
}

func (r *ReconcileCephMon) createOrUpdate(object runtime.Object) (reconcile.Result, error) {
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

// updatesMonMapOnPodStatus checks the status of our monitor pod using the provided podCheckFunc,
// if the function returns true the monCluster MonMap is updated.  Returns true iff the MonMap was updated.
func (r *ReconcileCephMon) updateMonMapOnPodStatus(monCluster *cephv1alpha1.CephMonCluster, instance *cephv1alpha1.CephMon, checkFunc podCheckFunc) (bool, error) {
	status, podIP, err := r.checkPod(instance.GetPodName(), instance.Namespace, checkFunc)
	if err != nil {
		return false, err
	}

	if !status {
		return false, nil
	}

	monMapEntry := cephv1alpha1.MonMapEntry{
		IP:         podIP,
		Port:       6789,
		StartEpoch: monCluster.Status.StartEpoch,
	}
	err = r.updateMonMap(monCluster, instance.Spec.ID, monMapEntry)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *ReconcileCephMon) updateMonMap(monCluster *cephv1alpha1.CephMonCluster, id string, e cephv1alpha1.MonMapEntry) error {
	monCluster.Status.MonMapUpdate(id, e)
	return r.client.Update(context.TODO(), monCluster)
}
