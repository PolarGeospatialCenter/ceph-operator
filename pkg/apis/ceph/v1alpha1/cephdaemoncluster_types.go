package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type CephDaemonClusterState string

const (
	CephDaemonClusterStateIdle    CephDaemonClusterState = "Idle"
	CephDaemonClusterStateRunning                        = "Running"
	CephDaemonClusterStateScaling                        = "Scaling"
	CephDaemonClusterStateError                          = "Error"
)

// CephDaemonClusterSpec defines the desired state of CephDaemonCluster
type CephDaemonClusterSpec struct {
	ClusterName           string         `json:"clusterName"`
	Image                 ImageSpec      `json:"image"`
	CephConfConfigMapName string         `json:"cephConfConfigMapName"`
	DaemonType            CephDaemonType `json:"daemonType"`
	Disabled              bool           `json:"disabled"`
	Replicas              int            `json:"replicas"`
}

// CephDaemonClusterStatus defines the observed state of CephDaemonCluster
type CephDaemonClusterStatus struct {
	State CephDaemonClusterState `json:"state"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephDaemonCluster is the Schema for the cephdaemonclusters API
// +k8s:openapi-gen=true
type CephDaemonCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CephDaemonClusterSpec   `json:"spec,omitempty"`
	Status CephDaemonClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephDaemonClusterList contains a list of CephDaemonCluster
type CephDaemonClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CephDaemonCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CephDaemonCluster{}, &CephDaemonClusterList{})
}

func NewCephDaemonCluster(t CephDaemonType) *CephDaemonCluster {
	return &CephDaemonCluster{Spec: CephDaemonClusterSpec{
		DaemonType: t,
	}}
}

func (d *CephDaemonCluster) GetState() CephDaemonClusterState {
	return d.Status.State
}

func (d *CephDaemonCluster) SetState(s CephDaemonClusterState) {
	d.Status.State = s
}

func (c *CephDaemonCluster) GetDaemonType() CephDaemonType {
	return c.Spec.DaemonType
}

func (c *CephDaemonCluster) SetCephClusterName(name string) {
	c.Spec.ClusterName = name
}

func (c *CephDaemonCluster) GetCephClusterName() string {
	return c.Spec.ClusterName
}

func (c *CephDaemonCluster) SetImage(image ImageSpec) {
	c.Spec.Image = image
}

func (c *CephDaemonCluster) GetImage() ImageSpec {
	return c.Spec.Image
}

func (c *CephDaemonCluster) SetCephConfConfigMapName(name string) {
	c.Spec.CephConfConfigMapName = name
}

func (c *CephDaemonCluster) GetCephConfConfigMapName() string {
	return c.Spec.CephConfConfigMapName
}

func (c *CephDaemonClusterList) AllInState(state CephDaemonClusterState) bool {
	for _, e := range c.Items {
		if e.GetState() != state {
			return false
		}
	}
	return true
}
