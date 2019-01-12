package v1alpha1

import (
	"bytes"
	"fmt"

	ini "gopkg.in/ini.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const MonitorServiceLabel = "ceph.k8s.pgc.umn.edu/monitorService"

// CephClusterSpec defines the desired state of CephCluster
type CephClusterSpec struct {
	Config         map[string]map[string]string `json:"config"`
	Fsid           string                       `json:"fsid"`
	MonServiceName string                       `json:"monServiceName"`
	ClusterDomain  string                       `json:"clusterDomain"`
	MonImage       ImageSpec                    `json:"monImage"`
	OsdImage       ImageSpec                    `json:"osdImage"`
	MgrImage       ImageSpec                    `json:"mgrImage"`
	MdsImage       ImageSpec                    `json:"mdsImage"`
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
	MonClusterName string `json:"monClusterName"`
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
	// FSID, Mon_Host, Public Network, Private Network, Osd

	cephConfIni := ini.Empty()

	global, err := cephConfIni.NewSection("global")
	if err != nil {
		return nil, err
	}

	_, err = global.NewKey("fsid", c.Spec.Fsid)
	if err != nil {
		return nil, err
	}

	_, err = global.NewKey("mon_host", c.Spec.MonServiceName)
	if err != nil {
		return nil, err
	}

	for sectionName, sectionMap := range c.Spec.Config {
		section, err := cephConfIni.NewSection(sectionName)
		if err != nil {
			return nil, err
		}
		for k, v := range sectionMap {
			_, err = section.NewKey(k, v)
		}
	}

	cephConf := bytes.NewBufferString("")
	cephConfIni.WriteTo(cephConf)

	cm := &corev1.ConfigMap{}
	cm.Name = c.GetCephConfigMapName()
	cm.Data = map[string]string{"ceph.conf": cephConf.String()}

	return cm, nil
}

func (c *CephCluster) GetCephConfigMapName() string {
	return fmt.Sprintf("ceph-%s-conf", c.GetName())
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

func (c *CephCluster) GetMonitorService() *corev1.Service {
	svc := &corev1.Service{}

	svc.Name = c.Spec.MonServiceName

	svc.Spec = corev1.ServiceSpec{
		Ports: []corev1.ServicePort{
			corev1.ServicePort{
				Name:       "ceph-mon",
				Port:       6789,
				TargetPort: intstr.FromInt(6789),
			},
		},
		Selector: map[string]string{
			MonitorServiceLabel: "",
		},
		ClusterIP: "None",
	}

	return svc
}

func (c *CephCluster) GetMonitorDiscoveryService() *corev1.Service {
	svc := &corev1.Service{}

	svc.Name = fmt.Sprintf("%s-discovery", c.Spec.MonServiceName)

	svc.Spec = corev1.ServiceSpec{
		Ports: []corev1.ServicePort{
			corev1.ServicePort{
				Name:       "ceph-mon",
				Port:       6789,
				TargetPort: intstr.FromInt(6789),
			},
		},
		Selector: map[string]string{
			MonitorServiceLabel: "",
		},
		ClusterIP:                "None",
		PublishNotReadyAddresses: true,
	}

	return svc
}
