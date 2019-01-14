package v1alpha1

import (
	"encoding/json"
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CephMonClusterSpec defines the desired state of CephMonCluster
type CephMonClusterSpec struct {
	ClusterName string `json:"clusterName"`
}

type JsonMonMap MonMap

type MonMap map[string]MonMapEntry

type MonMapEntry struct {
	PodName    string
	IP         net.IP
	Port       int
	StartEpoch int
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

type MonClusterState string

const (
	MonClusterIdle               MonClusterState = "Idle"
	MonClusterLaunching          MonClusterState = "Launching"
	MonClusterEstablishingQuorum MonClusterState = "Establishing Quorum"
	MonClusterInQuorum           MonClusterState = "In Quorum"
	MonClusterLostQuorum         MonClusterState = "Lost Quorum"
)

// CephMonClusterStatus defines the observed state of CephMonCluster
type CephMonClusterStatus struct {
	StartEpoch int             `json:"monStartEpoch"`
	State      MonClusterState `json:"monClusterState"`
	MonMap     MonMap          `json:"monMap"`
}

func (c *CephMonClusterStatus) GetMonMap() MonMap {
	if c.MonMap == nil {
		c.MonMap = make(MonMap)
	}
	return c.MonMap
}

func (c *CephMonClusterStatus) MonMapUpdate(id string, e MonMapEntry) {
	if c.MonMap == nil {
		c.MonMap = make(MonMap)
	}
	c.MonMap[id] = e
}

func (c *CephMonClusterStatus) MonMapEmpty() bool {
	return c.GetMonMap().Empty()
}

func (c *CephMonClusterStatus) MonMapQuorumCount() int {
	return c.GetMonMap().QuorumCount()
}

func (c *CephMonClusterStatus) MonMapContains(mon *CephMon) bool {
	return c.GetMonMap().Contains(mon)
}

func (c *CephMonClusterStatus) MonMapQuorumAtEpoch(epoch int) bool {
	return c.GetMonMap().QuorumAtEpoch(epoch)
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

func (c *CephMonCluster) GetMonMapConfigMap() (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}

	data := struct {
		StartEpoch int        `json:"startEpoch"`
		MonMap     JsonMonMap `json:"monMap"`
	}{
		StartEpoch: c.Status.StartEpoch,
		MonMap:     JsonMonMap(c.Status.MonMap),
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
