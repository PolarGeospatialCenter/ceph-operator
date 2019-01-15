package cephmon

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

type MonClusterEventMapper struct {
	client client.Client
	scheme *runtime.Scheme
}

func (m *MonClusterEventMapper) Map(o handler.MapObject) []reconcile.Request {
	req := make([]reconcile.Request, 0, 1)
	switch obj := o.Object.(type) {

	case *cephv1alpha1.CephMonCluster:

		monitors := &cephv1alpha1.CephMonList{}
		listOptions := &client.ListOptions{}
		listOptions.MatchingLabels(map[string]string{cephv1alpha1.MonitorClusterLabel: obj.Spec.ClusterName})

		err := m.client.List(context.TODO(), listOptions, monitors)
		if err != nil {
			logm.Error(err, "unable to list monitors for mon cluster")
			return req
		}

		for _, mon := range monitors.Items {
			req = append(req, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      mon.GetName(),
					Namespace: mon.GetNamespace(),
				},
			})
		}
	}

	return req
}
