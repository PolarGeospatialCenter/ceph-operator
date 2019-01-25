package cephdaemon

import (
	"context"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logm = logf.Log.WithName("controller_cephmon_monclustereventmapper")

type DaemonClusterEventMapper struct {
	client client.Client
	scheme *runtime.Scheme
}

func (m *DaemonClusterEventMapper) Map(o handler.MapObject) []reconcile.Request {
	req := make([]reconcile.Request, 0, 1)
	switch obj := o.Object.(type) {

	case *cephv1alpha1.CephDaemonCluster:

		daemons := &cephv1alpha1.CephDaemonList{}
		listOptions := &client.ListOptions{}
		listOptions.MatchingLabels(map[string]string{cephv1alpha1.ClusterNameLabel: obj.Spec.ClusterName,
			cephv1alpha1.DaemonTypeLabel: obj.Spec.DaemonType.String()})

		err := m.client.List(context.TODO(), listOptions, daemons)
		if err != nil {
			logm.Error(err, "unable to list monitors for mon cluster")
			return req
		}

		for _, d := range daemons.Items {
			req = append(req, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      d.GetName(),
					Namespace: d.GetNamespace(),
				},
			})
		}
	}

	return req
}
