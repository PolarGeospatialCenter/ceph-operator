package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CephMonSpec defines the desired state of CephMon
type CephMonSpec struct {
	ClusterName      string `json:"clusterName"`
	ID               string `json:"id"`
	PvSelectorString string `json:"pvSelectorString"`
	Disabled         bool   `json:"disabled"`
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

func (m *CephMon) GetVolumeClaimTemplate() (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	filesystemMode := corev1.PersistentVolumeFilesystem

	pvc.APIVersion = "v1"
	pvc.Kind = "PersistentVolumeCliam"

	pvc.Name = m.GetName()

	ls, err := m.GetPvLabelSelector()
	if err != nil {
		return nil, err
	}

	pvc.Spec.Selector = ls
	pvc.Spec.VolumeMode = &filesystemMode
	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteOnce,
	}
	qty := resource.NewQuantity(100000, resource.DecimalSI)
	pvc.Spec.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceStorage: *qty,
		},
	}

	return pvc, nil
}

func (m *CephMon) GetPod(monImage, cephConfConfigMap, discoveryServiceName, namespace, clusterDomain string) *corev1.Pod {
	pod := &corev1.Pod{}

	pod.APIVersion = "v1"
	pod.Kind = "Pod"

	pod.Name = fmt.Sprintf("ceph-%s-mon-%s", m.GetClusterName(), m.GetName())

	pod.SetLabels(map[string]string{MonitorServiceLabel: ""})

	container := corev1.Container{}
	container.Name = "ceph-mon"
	container.Image = monImage
	container.Env = []corev1.EnvVar{
		corev1.EnvVar{
			Name:  "CMD",
			Value: "start_mon",
		},
		corev1.EnvVar{
			Name: "MON_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
		corev1.EnvVar{
			Name:  "MON_ID",
			Value: m.Spec.ID,
		},
	}

	container.VolumeMounts = []corev1.VolumeMount{
		corev1.VolumeMount{
			Name:      "ceph-conf",
			MountPath: "/etc/ceph",
		},
	}

	pod.Spec.Containers = []corev1.Container{container}

	pod.Spec.Volumes = []corev1.Volume{
		corev1.Volume{
			Name: "ceph-osd-data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: m.GetName(),
				},
			},
		},
		corev1.Volume{
			Name: "ceph-conf",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cephConfConfigMap,
					},
				},
			},
		},
	}

	pod.Spec.DNSConfig = &corev1.PodDNSConfig{
		Searches: []string{
			fmt.Sprintf("%s.%s.svc.%s", discoveryServiceName, namespace, clusterDomain),
		},
	}

	return pod
}

func (m *CephMon) GetPvLabelSelector() (*metav1.LabelSelector, error) {
	return metav1.ParseToLabelSelector(m.Spec.PvSelectorString)

}

func (m *CephMon) GetAPIVersion() string {
	return m.APIVersion
}

func (m *CephMon) SetAPIVersion(version string) {
	m.APIVersion = version
}

func (m *CephMon) GetKind() string {
	return m.Kind
}

func (m *CephMon) SetKind(kind string) {
	m.Kind = kind
}
