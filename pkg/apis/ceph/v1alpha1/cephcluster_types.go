package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
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

func (i ImageSpec) String() string {
	return fmt.Sprintf("%s:%s", i.Registry, i.Tag)
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

//GetCephConfigMap returns a configmap containing a vaild ceph.conf file.
func (c *CephCluster) GetCephConfigMap() (*corev1.ConfigMap, error) {
	// Inject monitor service name
	return nil, nil
}

func (c *CephCluster) GetMonImage() string {
	return c.Spec.MonImage.String()
}

func (c *CephCluster) GetOsdImage() string {
	return c.Spec.OsdImage.String()
}

func (c *CephCluster) GetMgrImage() string {
	return c.Spec.MgrImage.String()
}

func (c *CephCluster) GetMdsImage() string {
	return c.Spec.MdsImage.String()
}