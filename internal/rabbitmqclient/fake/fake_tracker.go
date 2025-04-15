package fake

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

type MockTracker struct {
	MockTrack func(ctx context.Context, mg resource.Managed) error
}

func (fn MockTracker) Track(ctx context.Context, mg resource.Managed) error {
	return fn.MockTrack(ctx, mg)
}
