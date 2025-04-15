package rabbitmqmeta

import (
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	AnnotationKeyCrossplaneManaged = "rabbitmq.crossplane.io/managed"
	errResourceManagedFailed       = "external object is managed by other resource or import is required (external name: %s)"
)

func SetCrossplaneManaged(cr metav1.Object, externalName string) {
	meta.SetExternalName(cr, externalName)
	meta.AddAnnotations(cr, map[string]string{AnnotationKeyCrossplaneManaged: "true"})
}

func IsNotCrossplaneManaged(o metav1.Object) bool {
	return !isCrossplaneManaged(o)
}

func isCrossplaneManaged(o metav1.Object) bool {
	return o.GetAnnotations()[AnnotationKeyCrossplaneManaged] == "true"
}

func NewNotCrossplaneManagedError(externalName string) error {
	return errors.Wrap(errors.New(fmt.Sprintf(errResourceManagedFailed, externalName)), "")
}
