package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CephClusterSpec defines the desired state of CephCluster
type CephClusterSpec struct {
	Config         map[string]map[string]interface{} `json:"config"`
	MonServiceName string                            `json:"monServiceName"`
	MonImage       ImageSpec                         `json:"monImage"`
	OsdImage       ImageSpec                         `json:"osdImage"`
	MgrImage       ImageSpec                         `json:"mgrImage"`
	MdsImage       ImageSpec                         `json:"mdsImage"`
}

type ImageSpec struct {
	Registry string `json:"registry"`
	Tag      string `json:"tag"`
}

// CephClusterStatus defines the observed state of CephCluster
type CephClusterStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephCluster is the Schema for the cephclusters API
// +k8s:openapi-gen=true
type CephCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CephClusterSpec   `json:"spec,omitempty"`
	Status CephClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephClusterList contains a list of CephCluster
type CephClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CephCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CephCluster{}, &CephClusterList{})
}
