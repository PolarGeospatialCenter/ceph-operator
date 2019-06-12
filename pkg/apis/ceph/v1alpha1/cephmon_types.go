package v1alpha1

import (
	"fmt"
	"net"
	"strconv"

	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const MonQuorumPodCondition corev1.PodConditionType = corev1.PodReady

// CephMonSpec defines the desired state of CephMon
type CephMonSpec struct {
	ClusterName      string `json:"clusterName"`
	ID               string `json:"id"`
	PvSelectorString string `json:"pvSelectorString"`
	Disabled         bool   `json:"disabled"`
	Port             int    `json:"port"`
}

// MonState describes the state of the monitor
type MonState string

const (
	MonLaunchPod       CephDaemonState = "Launch Pod"
	MonWaitForPodRun                   = "Wait for Pod Run"
	MonWaitForPodReady                 = "Wait for Pod Ready"
	MonInQuorum                        = "In Quorum"
	MonError                           = "Error"
	MonCleanup                         = "Cleanup"
	MonIdle                            = "Idle"
)

// CephMonStatus defines the observed state of CephMon
type CephMonStatus struct {
	StartEpoch   int             `json:"startEpoch"`
	State        CephDaemonState `json:"monState"`
	PodIP        net.IP          `json:"podIP"`
	InitalMember bool            `json:"initalMember"`
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

func (m *CephMon) GetPodName() string {
	return fmt.Sprintf("ceph-%s", m.GetName())
}

func (m *CephMon) GetPod(monCluster *CephMonCluster, clientAdminKeyringName string) *corev1.Pod {
	pod := &corev1.Pod{}

	pod.APIVersion = "v1"
	pod.Kind = "Pod"

	pod.Name = m.GetPodName()

	pod.SetLabels(map[string]string{MonitorServiceLabel: ""})

	container := corev1.Container{}
	container.Name = "ceph-mon"
	container.Image = monCluster.GetImage().String()
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
		corev1.EnvVar{
			Name:  "CLUSTER",
			Value: m.Spec.ClusterName,
		},
		corev1.EnvVar{
			Name:  "MON_CLUSTER_START_EPOCH",
			Value: strconv.Itoa(monCluster.Status.StartEpoch),
		},
	}

	container.VolumeMounts = []corev1.VolumeMount{
		corev1.VolumeMount{
			Name:      "ceph-conf",
			MountPath: "/etc/ceph",
		},
		corev1.VolumeMount{
			Name:      "ceph-mon-data",
			MountPath: "/mon",
		},
		corev1.VolumeMount{
			Name:      "moncluster-configmap",
			MountPath: "/config/moncluster",
		},
		corev1.VolumeMount{
			Name:      "client-admin-keyring",
			MountPath: "/keyrings/client.admin",
		},
	}

	container.ImagePullPolicy = corev1.PullAlways

	handler := corev1.Handler{
		Exec: &corev1.ExecAction{
			Command: []string{
				"/ceph/bin/mon_health.sh",
			},
		},
	}

	container.ReadinessProbe = &corev1.Probe{
		Handler:             handler,
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
		FailureThreshold:    3,
		TimeoutSeconds:      5,
	}

	container.LivenessProbe = &corev1.Probe{
		Handler:             handler,
		InitialDelaySeconds: 30,
		PeriodSeconds:       30,
		TimeoutSeconds:      5,
		FailureThreshold:    2,
	}

	pod.Spec.Containers = []corev1.Container{container}

	pod.Spec.Volumes = []corev1.Volume{
		corev1.Volume{
			Name: "ceph-mon-data",
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
						Name: monCluster.GetCephConfConfigMapName(),
					},
				},
			},
		},
		corev1.Volume{
			Name: "moncluster-configmap",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: monCluster.GetConfigMapName(),
					},
				},
			},
		},
		corev1.Volume{
			Name: "client-admin-keyring",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: clientAdminKeyringName,
				},
			},
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

func (c *CephMon) GetState() CephDaemonState {
	return c.Status.State
}

func (c *CephMon) SetState(state CephDaemonState) {
	c.Status.State = state
}

func (c *CephMon) StateIn(state ...CephDaemonState) bool {

	for _, st := range state {
		if c.GetState() == st {
			return true
		}
	}

	return false
}

func (c *CephMon) GetPort() int {
	if c.Spec.Port == 0 {
		return 6789
	}
	return c.Spec.Port
}

func (c *CephMon) GetMonMapEntry() MonMapEntry {
	return MonMapEntry{
		IP:           c.Status.PodIP,
		Port:         c.GetPort(),
		StartEpoch:   c.Status.StartEpoch,
		State:        c.Status.State,
		InitalMember: c.Status.InitalMember,
		NamespacedName: types.NamespacedName{
			Name:      c.GetName(),
			Namespace: c.GetNamespace(),
		},
	}
}
