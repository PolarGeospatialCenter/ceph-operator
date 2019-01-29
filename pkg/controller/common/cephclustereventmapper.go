package common

import (
	"context"

	cephv1alpha1 "github.com/PolarGeospatialCenter/ceph-operator/pkg/apis/ceph/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logm = logf.Log.WithName("controller_cephcluster_eventmapper")

type CephClusterEventMapper struct {
	Client     client.Client
	Scheme     *runtime.Scheme
	ApiVersion string
	Kind       string
}

func (m *CephClusterEventMapper) Map(o handler.MapObject) []reconcile.Request {
	req := make([]reconcile.Request, 0, 1)
	switch obj := o.Object.(type) {

	case *cephv1alpha1.CephCluster:
		objects := &unstructured.UnstructuredList{}
		objects.SetKind(m.Kind)
		objects.SetAPIVersion(m.ApiVersion)
		listOptions := &client.ListOptions{}
		listOptions.MatchingLabels(map[string]string{cephv1alpha1.ClusterNameLabel: obj.GetName()})

		err := m.Client.List(context.TODO(), listOptions, objects)
		if err != nil {
			logm.Error(err, "unable to list daemon clusters for ceph cluster")
			return req
		}

		for _, obj := range objects.Items {
			req = append(req, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      obj.GetName(),
					Namespace: obj.GetNamespace(),
				},
			})
		}
	}

	return req
}
