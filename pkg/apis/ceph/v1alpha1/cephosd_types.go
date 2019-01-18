package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CephOsdSpec defines the desired state of CephOsd
type CephOsdSpec struct {
	ID               int    `json:"id"`
	ClusterName      string `json:"clusterName"`
	PvSelectorString string `json:"pvSelectorString"`
	Disabled         bool   `json:"disabled"`
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

func (o *CephOsd) GetVolumeClaimTemplate() (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	blockMode := corev1.PersistentVolumeBlock

	pvc.APIVersion = "v1"
	pvc.Kind = "PersistentVolumeCliam"

	pvc.Name = o.GetName()

	ls, err := o.GetPvLabelSelector()
	if err != nil {
		return nil, err
	}

	pvc.Spec.Selector = ls
	pvc.Spec.VolumeMode = &blockMode

	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteOnce,
	}
	storageClass := "local-storage"
	pvc.Spec.StorageClassName = &storageClass

	qty := resource.NewQuantity(100000, resource.DecimalSI)
	pvc.Spec.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceStorage: *qty,
		},
	}

	return pvc, nil
}

func (o *CephOsd) GetPodName() string {
	return fmt.Sprintf("ceph-%s-osd.%d", o.Spec.ClusterName, o.Spec.ID)
}

func (o *CephOsd) GetPod(osdImage, cephConfConfigMap, serviceAccountName string) *corev1.Pod {
	pod := &corev1.Pod{}

	pod.APIVersion = "v1"
	pod.Kind = "Pod"

	pod.Name = o.GetPodName()

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
			Value: o.Spec.ClusterName,
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

	container.ImagePullPolicy = corev1.PullAlways

	pod.Spec.ServiceAccountName = serviceAccountName

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

func (o *CephOsd) GetPvLabelSelector() (*metav1.LabelSelector, error) {
	return metav1.ParseToLabelSelector(o.Spec.PvSelectorString)

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
