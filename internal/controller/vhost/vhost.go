/*
Copyright 2022 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vhost

import (
	"context"
	"encoding/json"
	"fmt"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/provider-rabbitmq/apis"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/provider-rabbitmq/apis/core/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-rabbitmq/apis/v1alpha1"
	"github.com/crossplane/provider-rabbitmq/internal/features"
)

const (
	errNotVhost     = "managed resource is not a Vhost custom resource"
	errGetFailed    = "cannot get RabbitMq vhost"
	errCreateFailed = "cannot create RabbitMq vhost"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errNewClient = "cannot create new Service"
)

type RabbitMqService struct {
	rmqc *rabbithole.Client
}

type RabbitMqCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Endpoint string `json:"endpoint"`
}

var (
	newRabbitMqService = func(creds []byte) (*RabbitMqService, error) {
		var config = new(RabbitMqCredentials)
		if err := json.Unmarshal(creds, &config); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal credentials")
		}
		fmt.Printf("RabbitMq address: %s\n", config.Endpoint)
		c, err := rabbithole.NewClient(config.Endpoint, config.Username, config.Password)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create RabbitMQ client")
		}
		return &RabbitMqService{
			rmqc: c,
		}, err
	}
)

// Setup adds a controller that reconciles Vhost managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.VhostGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.VhostGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newServiceFn: newRabbitMqService}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Vhost{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(creds []byte) (*RabbitMqService, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Vhost)
	if !ok {
		return nil, errors.New(errNotVhost)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	cd := pc.Spec.Credentials
	data, err := resource.CommonCredentialExtractor(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	svc, err := c.newServiceFn(data)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{service: svc}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	service *RabbitMqService
}

// GenerateVhostObservation is used to produce v1alpha1.GroupGitLabObservation from gitlab.Group.
func GenerateVhostObservation(vh *rabbithole.VhostInfo) v1alpha1.VhostObservation {
	if vh == nil {
		return v1alpha1.VhostObservation{}
	}
	vhost := v1alpha1.VhostObservation{
		Name:                   vh.Name,
		Description:            &vh.Description,
		DefaultQueueType:       &vh.DefaultQueueType,
		Messages:               vh.Messages,
		MessagesReady:          vh.MessagesReady,
		MessagesUnacknowledged: vh.MessagesUnacknowledged,
	}
	return vhost
}

func GenerateClientVhostOptions(spec *v1alpha1.VhostSettings) rabbithole.VhostSettings {
	if spec == nil {
		return rabbithole.VhostSettings{}
	}
	settings := rabbithole.VhostSettings{}
	if spec.DefaultQueueType != nil {
		settings.DefaultQueueType = *spec.DefaultQueueType
	}
	if spec.Description != nil {
		settings.Description = *spec.Description
	}
	if spec.Tracing != nil {
		settings.Tracing = *spec.Tracing
	}
	if len(spec.Tags) > 0 {
		settings.Tags = make([]string, len(spec.Tags))
		copy(settings.Tags, spec.Tags)
	}
	return settings
}

func lateInitializeVhost(spec *v1alpha1.VhostParameters, api *rabbithole.VhostInfo) error {
	if api == nil {
		return nil
	}
	if spec.VhostSettings == nil {
		spec.VhostSettings = &v1alpha1.VhostSettings{}
	}
	if spec.VhostSettings.DefaultQueueType == nil {
		spec.VhostSettings.DefaultQueueType = &api.DefaultQueueType
	}
	if spec.VhostSettings.Description == nil {
		spec.VhostSettings.Description = &api.Description
	}
	if spec.VhostSettings.Tracing == nil {
		spec.VhostSettings.Tracing = &api.Tracing
	}
	if len(api.Tags) > 0 && len(spec.VhostSettings.Tags) > 0 {
		spec.VhostSettings.Tags = make([]string, len(api.Tags))
		copy(spec.VhostSettings.Tags, api.Tags)
	}
	return nil
}

func isVhostUpToDate(spec *v1alpha1.VhostParameters, api *rabbithole.VhostInfo) (bool, error) { //nolint:gocyclo
	if spec.VhostSettings != nil {
		if !apis.IsStringPtrEqualToString(spec.VhostSettings.Description, api.Description) {
			return false, nil
		}
		if !apis.IsStringPtrEqualToString(spec.VhostSettings.DefaultQueueType, api.DefaultQueueType) {
			return false, nil
		}
		if !apis.IsBoolPtrEqualToBool(spec.VhostSettings.Tracing, api.Tracing) {
			return false, nil
		}
		if !cmp.Equal([]string(spec.VhostSettings.Tags), []string(api.Tags), cmpopts.EquateEmpty()) {
			return false, nil
		}
	}
	return true, nil
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Vhost)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotVhost)
	}
	rmqVhost, err := c.service.rmqc.GetVhost(cr.Spec.ForProvider.HostName)
	if err != nil {
		var errResp rabbithole.ErrorResponse
		if errors.As(err, &errResp) && errResp.StatusCode == http.StatusNotFound {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}

	current := cr.Spec.ForProvider.DeepCopy()
	err = lateInitializeVhost(&cr.Spec.ForProvider, rmqVhost)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}
	isResourceLateInitialized := !cmp.Equal(current, &cr.Spec.ForProvider)

	cr.Status.AtProvider = GenerateVhostObservation(rmqVhost)
	cr.Status.SetConditions(xpv1.Available())

	isUpToDate, err := isVhostUpToDate(&cr.Spec.ForProvider, rmqVhost)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}
	fmt.Printf("IsUpToDate: %v, LateInitializeVhost: %v\n", isUpToDate, isResourceLateInitialized)

	return managed.ExternalObservation{
		// Return false when the external resource does not exist. This lets
		// the managed resource reconciler know that it needs to call Create to
		// (re)create the resource, or that it has successfully been deleted.
		ResourceExists: true,
		// Return false when the external resource exists, but it not up to date
		// with the desired managed resource state. This lets the managed
		// resource reconciler know that it needs to call Update.
		ResourceUpToDate: isUpToDate,
		// ResourceLateInitialized should be true if the managed resource's spec was
		// updated during its observation.
		ResourceLateInitialized: isResourceLateInitialized,
		// Return any details that may be required to connect to the external
		// resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Vhost)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotVhost)
	}
	fmt.Printf("Creating vhost: %+v", cr.Spec.ForProvider.HostName)
	_, err := c.service.rmqc.PutVhost(cr.Spec.ForProvider.HostName, GenerateClientVhostOptions(cr.Spec.ForProvider.VhostSettings))
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Vhost)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotVhost)
	}

	fmt.Printf("Updating vhost: %+v\n", cr.Spec.ForProvider.HostName)
	options := GenerateClientVhostOptions(cr.Spec.ForProvider.VhostSettings)
	_, err := c.service.rmqc.PutVhost(cr.Spec.ForProvider.HostName, options)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errCreateFailed)
	}

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Vhost)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotVhost)
	}
	fmt.Printf("Deleting vhost: %+v", cr.Spec.ForProvider.HostName)
	_, err := c.service.rmqc.DeleteVhost(cr.Spec.ForProvider.HostName)
	if err != nil {
		fmt.Printf("Error deleting Vhost: %+v\n", err)
		return managed.ExternalDelete{}, errors.Wrap(err, errCreateFailed)
	}
	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	return nil
}
