package common

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type ownerObject interface {
	metav1.Type
	GetUID() types.UID
	GetName() string
}

type ownableObject interface {
	SetOwnerReferences([]metav1.OwnerReference)
	GetOwnerReferences() []metav1.OwnerReference
}

// UpdateOwnerReferences updates the OwnerReferences field to reference this PersistentVolumeSet
func UpdateOwnerReferences(owner ownerObject, owned ownableObject) error {
	if owner.GetUID() == "" {
		return fmt.Errorf("uid must not be set")
	}

	or := &metav1.OwnerReference{}
	or.APIVersion = owner.GetAPIVersion()
	or.Kind = owner.GetKind()
	or.Name = owner.GetName()
	or.UID = owner.GetUID()

	owned.SetOwnerReferences(append(owned.GetOwnerReferences(), *or))
	return nil
}
