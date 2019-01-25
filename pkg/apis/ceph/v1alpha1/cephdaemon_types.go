package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/rand"
)

type CephDaemonState string

type CephDaemonType string

func (c CephDaemonType) String() string {
	return string(c)
}

const (
	CephDaemonTypeMgr CephDaemonType = "mgr"
	CephDaemonTypeMds CephDaemonType = "mds"
	CephDaemonTypeRgw CephDaemonType = "rgw"
)

// CephDaemonSpec defines the desired state of CephDaemon
type CephDaemonSpec struct {
	ClusterName           string         `json:"clusterName"`
	ID                    string         `json:"id"`
	Image                 ImageSpec      `json:"image"`
	CephConfConfigMapName string         `json:"cephConfConfigMapName"`
	DaemonType            CephDaemonType `json:"daemonType"`
	Disabled              bool           `json:"disabled"`
}

// CephDaemonStatus defines the observed state of CephDaemon
type CephDaemonStatus struct {
	State CephDaemonState `json:"state"`
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
	// Always have non-numeric start char
	c := 'a' + rand.Intn(26)
	d.Spec.ID = string(c) + rand.String(5)
	d.Spec.ClusterName = clusterName
	d.Name = fmt.Sprintf("ceph-%s-%s.%s", clusterName, string(t), d.Spec.ID)

	return d
}

//CheckReady returns true if the daemon is ready
func (d *CephDaemon) CheckReady() bool {
	return false
}

func (d *CephDaemon) GetState() CephDaemonState {
	return d.Status.State
}

func (d *CephDaemon) SetState(s CephDaemonState) {
	d.Status.State = s
}

func (d *CephDaemon) GetPodName() string {
	return fmt.Sprintf("ceph-%s-%s.%s", d.Spec.ClusterName, string(d.Spec.DaemonType), d.Spec.ID)
}

//
// func (d *CephDaemon) GetPod() (*corev1.Pod, error) {
// 	pod := d.getBasePod()
//
// 	var volumeMounts []corev1.VolumeMount
// 	var volumes []corev1.Volume
//
// 	envs := []corev1.EnvVar{corev1.EnvVar{
// 		Name:  "CMD",
// 		Value: fmt.Sprintf("start_%s", d.Spec.DaemonType),
// 	}}
//
// 	volumeMounts := []corev1.VolumeMount{corev1.VolumeMount{
// 		Name:      fmt.Sprintf("ceph-%s-bootstrap-keyring"),
// 		MountPath: "/etc/ceph",
// 	}}
//
// 	switch d.Spec.DaemonType {
// 	case CephDaemonTypeMgr:
//
// 	default:
// 		return nil, fmt.Errorf("daemontype %s not supported for pod generation", d.Spec.DaemonType)
// 	}
//
// 	for _, env := range envs {
// 		pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, env)
// 	}
//
// 	return pod, nil
// }

func (d *CephDaemon) GetBasePod() *corev1.Pod {
	pod := &corev1.Pod{}

	pod.Name = d.GetPodName()
	pod.Namespace = d.GetNamespace()

	pod.SetLabels(map[string]string{
		ClusterNameLabel: d.Spec.ClusterName,
		DaemonTypeLabel:  d.Spec.DaemonType.String(),
	})

	container := corev1.Container{}
	container.Name = fmt.Sprintf("ceph-%s", d.Spec.DaemonType.String())
	container.Image = d.Spec.Image.String()
	container.Env = []corev1.EnvVar{
		corev1.EnvVar{
			Name:  "CLUSTER",
			Value: d.Spec.ClusterName,
		},
	}
	container.VolumeMounts = []corev1.VolumeMount{
		corev1.VolumeMount{
			Name:      "ceph-conf",
			MountPath: "/etc/ceph",
		},
	}

	// Fix this
	container.ImagePullPolicy = corev1.PullAlways

	pod.Spec.Containers = []corev1.Container{container}

	pod.Spec.Volumes = []corev1.Volume{
		corev1.Volume{
			Name: "ceph-conf",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: d.Spec.CephConfConfigMapName,
					},
				},
			},
		},
	}

	return pod
}
