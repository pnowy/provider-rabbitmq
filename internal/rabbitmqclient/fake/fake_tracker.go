package fake

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockApplicator struct {
}

func (m MockApplicator) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return nil
}

func (m MockApplicator) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return nil
}

func (m MockApplicator) Apply(_ context.Context, _ runtime.ApplyConfiguration, _ ...client.ApplyOption) error {
	return nil
}

func (m MockApplicator) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	return nil
}

func (m MockApplicator) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	return nil
}

func (m MockApplicator) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	return nil
}

func (m MockApplicator) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
	return nil
}

func (m MockApplicator) DeleteAllOf(_ context.Context, _ client.Object, _ ...client.DeleteAllOfOption) error {
	return nil
}

func (m MockApplicator) Status() client.SubResourceWriter {
	return nil
}

func (m MockApplicator) SubResource(_ string) client.SubResourceClient {
	return nil
}

func (m MockApplicator) Scheme() *runtime.Scheme {
	return nil
}

func (m MockApplicator) RESTMapper() meta.RESTMapper {
	return nil
}

func (m MockApplicator) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}

func (m MockApplicator) IsObjectNamespaced(_ runtime.Object) (bool, error) {
	return true, nil
}
