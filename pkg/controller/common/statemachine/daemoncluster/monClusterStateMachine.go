package daemoncluster

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"github.com/PolarGeospatialCenter/ceph-operator/pkg/controller/common/keyrings"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MonClusterStateMachine struct {
	*BaseStateMachine
}

func (s *MonClusterStateMachine) currentStartEpoch() int {
	monCluster := s.daemonCluster.(*cephv1alpha1.CephMonCluster)
	return monCluster.Status.StartEpoch
}

func (s *MonClusterStateMachine) GetTransition(readClient ReadOnlyClient) (TransitionFunc, cephv1alpha1.CephDaemonClusterState) {

	if !s.cluster.GetDaemonEnabled(cephv1alpha1.CephDaemonTypeMon) &&
		(s.daemonCluster.GetState() != cephv1alpha1.MonClusterIdle ||
			s.daemonCluster.GetState() != cephv1alpha1.MonClusterLostQuorum) {
		return nil, cephv1alpha1.MonClusterLostQuorum
	}

	if !s.cluster.GetDaemonEnabled(cephv1alpha1.CephDaemonTypeMon) &&
		s.daemonCluster.GetState() == cephv1alpha1.MonClusterIdle {
		return nil, s.State()
	}

	fullMonMap, err := s.getMonMap(readClient)
	if err != nil {
		return s.emitError(err), s.State()
	}

	monMap := fullMonMap.GetInitalMonMap()

	switch s.State() {

	case cephv1alpha1.MonClusterGenKeyrings:
		return s.createKeyrings, cephv1alpha1.MonClusterIdle

	case cephv1alpha1.MonClusterGenMonMap:
		return s.generateMonMap, cephv1alpha1.MonClusterIdle

	case cephv1alpha1.MonClusterIdle:

		keysExist, err := s.keyringsExist(readClient)
		if err != nil {
			return s.emitError(err), s.State()
		}
		if !keysExist {
			return nil, cephv1alpha1.MonClusterGenKeyrings
		}

		monMapExists, err := s.monMapExists(readClient)
		if err != nil {
			return s.emitError(err), s.State()
		}
		if !monMapExists {
			return nil, cephv1alpha1.MonClusterGenMonMap
		}

		if !fullMonMap.Empty() {
			return s.incrementClusterEpoch, cephv1alpha1.MonClusterLaunching
		}

	case cephv1alpha1.MonClusterLaunching:
		if fullMonMap.CountInitalMembers() < 1 {
			return nil, cephv1alpha1.MonClusterEnableFirstMon
		}

		if monMap.QuorumAtEpoch(s.currentStartEpoch()) {
			return s.generateMonMap, cephv1alpha1.MonClusterEstablishingQuorum
		}

	case cephv1alpha1.MonClusterEnableFirstMon:
		return s.enableFirst, cephv1alpha1.MonClusterLaunching

	case cephv1alpha1.MonClusterEstablishingQuorum:

		totalInQuorum := monMap.CountInState(cephv1alpha1.MonInQuorum)
		if totalInQuorum >= monMap.QuorumCount() {
			return nil, cephv1alpha1.MonClusterInQuorum
		}
		totalInIdle := monMap.CountInState(cephv1alpha1.MonIdle)
		if totalInIdle >= monMap.QuorumCount() {
			return nil, cephv1alpha1.MonClusterLostQuorum
		}

	case cephv1alpha1.MonClusterInQuorum:

		totalInQuorum := monMap.CountInState(cephv1alpha1.MonInQuorum)
		if totalInQuorum < monMap.QuorumCount() {
			return nil, cephv1alpha1.MonClusterLostQuorum
		}

	case cephv1alpha1.MonClusterLostQuorum:

		if monMap.AllInState(cephv1alpha1.MonIdle) {
			return nil, cephv1alpha1.MonClusterIdle
		}

	default:
		return nil, cephv1alpha1.MonClusterLostQuorum
	}
	return nil, s.State()
}

func (s *MonClusterStateMachine) keyringsExist(readClient ReadOnlyClient) (bool, error) {
	return false, nil
}

func (s *MonClusterStateMachine) monMapExists(readClient ReadOnlyClient) (bool, error) {
	return false, nil
}

func (s *MonClusterStateMachine) getMonMap(readClient ReadOnlyClient) (cephv1alpha1.MonMap, error) {
	monitors := &cephv1alpha1.CephMonList{}
	listOptions := &client.ListOptions{}
	listOptions.MatchingLabels(map[string]string{cephv1alpha1.ClusterNameLabel: s.cluster.GetName()})

	err := readClient.List(context.TODO(), listOptions, monitors)
	if err != nil {
		return nil, err
	}

	monMap := make(cephv1alpha1.MonMap, len(monitors.Items))

	for _, mon := range monitors.Items {
		monMap[mon.Spec.ID] = mon.GetMonMapEntry()
	}

	return monMap, nil
}

func (s *MonClusterStateMachine) generateMonMap(c client.Client, scheme *runtime.Scheme) error {
	fullMonMap, err := s.getMonMap(c)
	if err != nil {
		return err
	}

	cm, err := s.GetMonMapConfigMap(fullMonMap.GetInitalMonMap())
	if err != nil {
		return err
	}
	cm.Namespace = s.daemonCluster.GetNamespace()

	err = c.Create(context.TODO(), cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	} else if err == nil {
		return nil
	}

	return c.Update(context.TODO(), cm)
}

func (s *MonClusterStateMachine) createKeyrings(c client.Client, scheme *runtime.Scheme) error {
	// Create secrets, new state?
	for _, k := range []keyrings.Keyring{keyrings.MON_KEYRING, keyrings.CLIENT_ADMIN_KEYRING} {
		secret := &corev1.Secret{}
		secretNamespacedName := &types.NamespacedName{
			Namespace: s.daemonCluster.GetNamespace(),
			Name:      k.GetSecretName(s.cluster.GetName()),
		}
		err := c.Get(context.TODO(), *secretNamespacedName, secret)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}

		if errors.IsNotFound(err) {
			err = k.GenerateKey()
			if err != nil {
				return err
			}

			secret = k.GetSecret(s.cluster.GetName())
			secret.Namespace = s.daemonCluster.GetNamespace()
			err = c.Create(context.TODO(), secret)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *MonClusterStateMachine) incrementClusterEpoch(c client.Client, scheme *runtime.Scheme) error {
	monCluster := s.daemonCluster.(*cephv1alpha1.CephMonCluster)
	monCluster.Status.StartEpoch++
	return c.Update(context.TODO(), monCluster)
}

func (s *MonClusterStateMachine) enableFirst(c client.Client, scheme *runtime.Scheme) error {
	fullMonMap, err := s.getMonMap(c)
	if err != nil {
		return err
	}

	initialMonNamespacedName := fullMonMap.GetRandomEntry().NamespacedName
	initialMon := &cephv1alpha1.CephMon{}

	err = c.Get(context.TODO(), initialMonNamespacedName, initialMon)
	if err != nil {
		return err
	}

	initialMon.Status.InitalMember = true
	return c.Update(context.TODO(), initialMon)
}

func (s *MonClusterStateMachine) GetMonMapConfigMap(monMap cephv1alpha1.MonMap) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}

	data := struct {
		StartEpoch int                     `json:"startEpoch"`
		MonMap     cephv1alpha1.JsonMonMap `json:"monMap"`
	}{
		StartEpoch: s.currentStartEpoch(),
		MonMap:     cephv1alpha1.JsonMonMap(monMap.GetInitalMonMap()),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	monCluster := s.daemonCluster.(*cephv1alpha1.CephMonCluster)
	cm.Name = monCluster.GetConfigMapName()
	cm.Data = map[string]string{
		"jsonMonMap": string(jsonData),
	}

	return cm, nil
}
