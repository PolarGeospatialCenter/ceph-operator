package cephmoncluster

import (
	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type MonEventMapper struct{}

func (m *MonEventMapper) Map(o handler.MapObject) []reconcile.Request {
	req := make([]reconcile.Request, 0, 1)
	switch obj := o.Object.(type) {

	case *cephv1alpha1.CephMon:
		req = append(req, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      obj.Spec.ClusterName,
				Namespace: obj.Namespace,
			},
		})
	}

	return req
}
