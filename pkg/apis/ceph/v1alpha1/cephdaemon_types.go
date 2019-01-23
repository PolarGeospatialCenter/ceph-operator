package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

// CephDaemonSpec defines the desired state of CephDaemon
type CephDaemonSpec struct {
	ClusterName           string         `json:"clusterName"`
	ID                    string         `json:"id"`
	Image                 ImageSpec      `json:"image"`
	CephConfConfigMapName string         `json:"cephConfConfigMapName"`
	DaemonType            CephDaemonType `json:"daemonType"`
}

// CephDaemonStatus defines the observed state of CephDaemon
type CephDaemonStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephDaemon is the Schema for the cephdaemons API
// +k8s:openapi-gen=true
type CephDaemon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CephDaemonSpec   `json:"spec,omitempty"`
	Status CephDaemonStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephDaemonList contains a list of CephDaemon
type CephDaemonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CephDaemon `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CephDaemon{}, &CephDaemonList{})
}

func NewCephDaemon(t CephDaemonType, clusterName string) *CephDaemon {
	d := &CephDaemon{}
	d.Spec.DaemonType = t
	d.Spec.ID = rand.String(6)
	d.Spec.ClusterName = clusterName
	d.Name = fmt.Sprintf("ceph-%s-%s.%s", clusterName, string(t), d.Spec.ID)

	return d
}
