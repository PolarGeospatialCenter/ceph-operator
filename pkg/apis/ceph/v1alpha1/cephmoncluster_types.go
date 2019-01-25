package v1alpha1

import (
	"encoding/json"
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type JsonMonMap MonMap

type MonMap map[string]MonMapEntry

type MonMapEntry struct {
	IP             net.IP
	Port           int
	StartEpoch     int
	State          MonState
	InitalMember   bool
	NamespacedName types.NamespacedName
}

func (m JsonMonMap) MarshalJSON() ([]byte, error) {
	out := make([]map[string]string, 0, len(m))

	for k, v := range m {
		out = append(out, map[string]string{
			"id":   k,
			"port": fmt.Sprintf("%d", v.Port),
			"ip":   v.IP.String(),
		})
	}

	return json.Marshal(out)
}

func (m MonMap) Contains(mon *CephMon) bool {
	_, ok := m[mon.Spec.ID]

	return ok
}

func (m MonMap) Empty() bool {
	return len(m) == 0
}

func (m MonMap) QuorumAtEpoch(epoch int) bool {
	atEpoch := 0

	for _, mon := range m {
		if mon.StartEpoch == epoch {
			atEpoch++
		}
	}

	return atEpoch >= m.QuorumCount()
}

func (m MonMap) QuorumCount() int {
	return (len(m) / 2) + 1
}

func (m MonMap) AllInState(state MonState) bool {
	for _, e := range m {
		if e.State != state {
			return false
		}
	}
	return true
}

func (m MonMap) CountInState(state MonState) int {
	var count int
	for _, e := range m {
		if e.State == state {
			count++
		}
	}
	return count
}

func (m MonMap) CountInitalMembers() int {
	var count int
	for _, e := range m {
		if e.InitalMember {
			count++
		}
	}
	return count
}

func (m MonMap) GetRandomEntry() MonMapEntry {
	for _, e := range m {
		return e
	}

	return MonMapEntry{}
}

func (m MonMap) GetInitalMonMap() MonMap {
	initMonMap := make(MonMap)

	for k, v := range m {
		if v.InitalMember {
			initMonMap[k] = v
		}
	}

	return initMonMap
}

type MonClusterState string

const (
	MonClusterIdle               MonClusterState = "Idle"
	MonClusterLaunching          MonClusterState = "Launching"
	MonClusterEstablishingQuorum MonClusterState = "Establishing Quorum"
	MonClusterInQuorum           MonClusterState = "In Quorum"
	MonClusterLostQuorum         MonClusterState = "Lost Quorum"
)

// CephMonClusterSpec defines the desired state of CephMonCluster
type CephMonClusterSpec struct {
	ClusterName           string    `json:"clusterName"`
	Image                 ImageSpec `json:"image"`
	CephConfConfigMapName string    `json:"cephConfConfigMapName"`
}

// CephMonClusterStatus defines the observed state of CephMonCluster
type CephMonClusterStatus struct {
	StartEpoch int             `json:"monStartEpoch"`
	State      MonClusterState `json:"monClusterState"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephMonCluster is the Schema for the cephmonclusters API
// +k8s:openapi-gen=true
type CephMonCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CephMonClusterSpec   `json:"spec,omitempty"`
	Status CephMonClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephMonClusterList contains a list of CephMonCluster
type CephMonClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CephMonCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CephMonCluster{}, &CephMonClusterList{})
}

func (c *CephMonCluster) GetMonMapConfigMap(monMap MonMap) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}

	data := struct {
		StartEpoch int        `json:"startEpoch"`
		MonMap     JsonMonMap `json:"monMap"`
	}{
		StartEpoch: c.Status.StartEpoch,
		MonMap:     JsonMonMap(monMap.GetInitalMonMap()),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	cm.Name = c.GetConfigMapName()
	cm.Data = map[string]string{
		"jsonMonMap": string(jsonData),
	}

	return cm, nil
}

func (c *CephMonCluster) GetConfigMapName() string {
	return fmt.Sprintf("%s-monmap", c.GetName())
}

func (c *CephMonCluster) GetMonClusterState() MonClusterState {
	return c.Status.State
}

func (c *CephMonCluster) SetMonClusterState(state MonClusterState) {
	c.Status.State = state
}

func (c *CephMonCluster) CheckMonClusterState(state ...MonClusterState) bool {

	for _, st := range state {
		if c.GetMonClusterState() == st {
			return true
		}
	}

	return false
}

func (c *CephMonCluster) SetCephClusterName(name string) {
	c.Spec.ClusterName = name
}

func (c *CephMonCluster) GetCephClusterName() string {
	return c.Spec.ClusterName
}

func (c *CephMonCluster) SetImage(image ImageSpec) {
	c.Spec.Image = image
}

func (c *CephMonCluster) GetImage() ImageSpec {
	return c.Spec.Image
}

func (c *CephMonCluster) SetCephConfConfigMapName(name string) {
	c.Spec.CephConfConfigMapName = name
}

func (c *CephMonCluster) GetCephConfConfigMapName() string {
	return c.Spec.CephConfConfigMapName
}

func (c *CephMonCluster) GetDaemonType() CephDaemonType {
	return CephDaemonType("mon")
}
