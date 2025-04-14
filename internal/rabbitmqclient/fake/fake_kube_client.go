package fake

import (
	"context"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

type MockKubeReaderClient struct {
}

func (k MockKubeReaderClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOptions) error {
	return nil
}

func (k MockKubeReaderClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}
