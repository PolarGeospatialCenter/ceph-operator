package v1alpha1

import (
	"testing"

	"github.com/go-test/deep"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetCephConfigMap(t *testing.T) {
	testCases := []struct {
		Name              string
		Cluster           CephCluster
		ExpectedConfigMap *corev1.ConfigMap
	}{
		{
			Name: "basic-config-map",
			Cluster: CephCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: CephClusterSpec{
					Fsid:           "FCA3CCCA-8258-4A72-8C10-39CF2B0585EE",
					MonServiceName: "monitor",
				},
			},
			ExpectedConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ceph-test-conf",
				},
				Data: map[string]string{
					"test.conf": "[global]\n" +
						"fsid     = FCA3CCCA-8258-4A72-8C10-39CF2B0585EE\n" +
						"mon_host = monitor\n\n"}},
		},
		{
			Name: "override-config-parameters",
			Cluster: CephCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: CephClusterSpec{
					Fsid:           "FCA3CCCA-8258-4A72-8C10-39CF2B0585EE",
					MonServiceName: "monitor",
					Config: map[string]map[string]string{
						"global": map[string]string{
							"mon_host": "fooservice",
							"keyring":  "/keyrings/client.admin/keyring",
						},
						"mon": map[string]string{
							"fookey": "barval",
						},
					},
				},
			},
			ExpectedConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ceph-test-conf",
				},
				Data: map[string]string{
					"test.conf": "[global]\n" +
						"fsid     = FCA3CCCA-8258-4A72-8C10-39CF2B0585EE\n" +
						"mon_host = fooservice\n" +
						"keyring  = /keyrings/client.admin/keyring\n" +
						"\n" +
						"[mon]\n" +
						"fookey = barval\n" +
						"\n"}},
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(st *testing.T) {
			cm, err := c.Cluster.GetCephConfigMap()
			if err != nil {
				st.Fatalf("Error getting config map: %v", err)
			}
			if diff := deep.Equal(cm, c.ExpectedConfigMap); len(diff) > 0 {
				st.Error("Resulting config map not equal to expected.")
				for _, l := range diff {
					st.Error(l)
				}
			}
		})
	}

}
