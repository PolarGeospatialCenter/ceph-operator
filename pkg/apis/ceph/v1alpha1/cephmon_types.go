package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CephMonSpec defines the desired state of CephMon
type CephMonSpec struct {
	ClusterName string               `json:"clusterName"`
	PvSelector  metav1.LabelSelector `json:"pvSelector"`
	Disabled    bool                 `json:"disabled"`
}

// CephMonStatus defines the observed state of CephMon
type CephMonStatus struct {
	Healthy bool `json:"healthy"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephMon is the Schema for the cephmons API
// +k8s:openapi-gen=true
type CephMon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CephMonSpec   `json:"spec,omitempty"`
	Status CephMonStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephMonList contains a list of CephMon
type CephMonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CephMon `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CephMon{}, &CephMonList{})
}

func (m *CephMon) GetDisabled() bool {
	return m.Spec.Disabled
}
