package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CephOsdSpec defines the desired state of CephOsd
type CephOsdSpec struct {
	ID          int                  `json:"id"`
	ClusterName string               `json:"clusterName"`
	PvSelector  metav1.LabelSelector `json:"pvSelector"`
	Disabled    bool                 `json:"disabled"`
}

// CephOsdStatus defines the observed state of CephOsd
type CephOsdStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephOsd is the Schema for the cephosds API
// +k8s:openapi-gen=true
type CephOsd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CephOsdSpec   `json:"spec,omitempty"`
	Status CephOsdStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephOsdList contains a list of CephOsd
type CephOsdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CephOsd `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CephOsd{}, &CephOsdList{})
}
