package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
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

func (o *CephOsd) GetDisabled() bool {
	return o.Spec.Disabled
}

func (o *CephOsd) GetVolumeClaimTemplate() *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{}
	blockMode := corev1.PersistentVolumeBlock

	pvc.APIVersion = "v1"
	pvc.Kind = "PersistentVolumeCliam"

	pvc.Name = o.GetName()

	pvc.Spec.Selector = &o.Spec.PvSelector
	pvc.Spec.VolumeMode = &blockMode

	return pvc
}

func (o *CephOsd) GetPod(osdImage, cephConfConfigMap string) *corev1.Pod {
	pod := &corev1.Pod{}

	pod.APIVersion = "v1"
	pod.Kind = "Pod"

	pod.Name = fmt.Sprintf("ceph-%s-osd.%d", o.GetClusterName(), o.Spec.ID)

	container := corev1.Container{}
	container.Name = "ceph-osd"
	container.Image = osdImage
	container.Env = []corev1.EnvVar{
		corev1.EnvVar{
			Name:  "CMD",
			Value: "start_osd",
		},
		corev1.EnvVar{
			Name:  "CLUSTER",
			Value: o.GetClusterName(),
		},
		corev1.EnvVar{
			Name: "NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			},
		},
	}

	container.VolumeDevices = []corev1.VolumeDevice{
		corev1.VolumeDevice{
			Name:       "ceph-osd-data",
			DevicePath: "/dev/osd",
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
					ClaimName: o.GetName(),
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

	return pod
}

func (o *CephOsd) GetAPIVersion() string {
	return o.APIVersion
}

func (o *CephOsd) SetAPIVersion(version string) {
	o.APIVersion = version
}

func (o *CephOsd) GetKind() string {
	return o.Kind
}

func (o *CephOsd) SetKind(kind string) {
	o.Kind = kind
}
