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

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqmeta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/pnowy/provider-rabbitmq/apis/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/features"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
)

const (
	errNotVhost     = "managed resource is not a Vhost custom resource"
	errGetFailed    = "cannot get RabbitMq vhost"
	errCreateFailed = "cannot create RabbitMq vhost"
	errUpdateFailed = "cannot update RabbitMq vhost"
	errDeleteFailed = "cannot delete RabbitMq vhost"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errNewClient = "cannot create new Service"
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
			newServiceFn: rabbitmqclient.NewClient,
			logger:       o.Logger.WithValues("controller", name)}),
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
	newServiceFn func(creds []byte) (*rabbitmqclient.RabbitMqService, error)
	logger       logging.Logger
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

	return &external{service: svc, log: c.logger}, nil
}

type external struct {
	service *rabbitmqclient.RabbitMqService
	log     logging.Logger
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Vhost)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotVhost)
	}
	name := getExternalName(cr)
	rmqVhost, err := c.service.Rmqc.GetVhost(name)
	if err != nil {
		if rabbitmqclient.IsNotFoundError(err) {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}
	if rabbitmqmeta.IsNotCrossplaneManaged(cr) {
		return managed.ExternalObservation{}, rabbitmqmeta.NewNotCrossplaneManagedError(name)
	}

	current := cr.Spec.ForProvider.DeepCopy()
	lateInitializeVhost(&cr.Spec.ForProvider, rmqVhost)
	isResourceLateInitialized := !cmp.Equal(current, &cr.Spec.ForProvider)

	cr.Status.AtProvider = generateVhostObservation(rmqVhost)
	cr.Status.SetConditions(xpv1.Available())

	isUpToDate := isVhostUpToDate(&cr.Spec.ForProvider, rmqVhost)
	c.log.Info("Reconciling vhost", "vhost", name, "upToDate", isUpToDate, "lateInitialized", isResourceLateInitialized)

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        isUpToDate,
		ResourceLateInitialized: isResourceLateInitialized,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Vhost)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotVhost)
	}

	name := getExternalName(cr)
	c.log.Info("Creating vhost", "vhost", name)
	resp, err := c.service.Rmqc.PutVhost(name, generateApiVhostSettings(cr.Spec.ForProvider.VhostSettings))
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		c.log.Debug("Error closing response body", "err", err)
	}
	rabbitmqmeta.SetCrossplaneManaged(cr, name)
	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Vhost)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotVhost)
	}
	name := getExternalName(cr)
	c.log.Info("Updating vhost", "vhost", name)
	options := generateApiVhostSettings(cr.Spec.ForProvider.VhostSettings)
	resp, err := c.service.Rmqc.PutVhost(name, options)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		c.log.Debug("Error closing response body", "err", err)
	}

	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Vhost)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotVhost)
	}

	name := getExternalName(cr)
	c.log.Info("Deleting vhost", "vhost", name)
	resp, err := c.service.Rmqc.DeleteVhost(name)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteFailed)
	}
	if err := resp.Body.Close(); err != nil {
		c.log.Debug("Error closing response body", "err", err)
	}
	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func getExternalName(spec *v1alpha1.Vhost) string {
	forProviderName := spec.Spec.ForProvider.HostName
	if forProviderName != nil {
		return *forProviderName
	}
	return spec.Name
}

// generateVhostObservation is used to produce v1alpha1.GroupGitLabObservation from gitlab.Group.
func generateVhostObservation(vh *rabbithole.VhostInfo) v1alpha1.VhostObservation {
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

func generateApiVhostSettings(spec *v1alpha1.VhostSettings) rabbithole.VhostSettings {
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

func lateInitializeVhost(spec *v1alpha1.VhostParameters, api *rabbithole.VhostInfo) {
	if api == nil {
		return
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
}

func isVhostUpToDate(spec *v1alpha1.VhostParameters, api *rabbithole.VhostInfo) bool { //nolint:gocyclo
	if spec.VhostSettings != nil {
		if !rabbitmqclient.IsStringPtrEqualToString(spec.VhostSettings.Description, api.Description) {
			return false
		}
		if !rabbitmqclient.IsStringPtrEqualToString(spec.VhostSettings.DefaultQueueType, api.DefaultQueueType) {
			return false
		}
		if !rabbitmqclient.IsBoolPtrEqualToBool(spec.VhostSettings.Tracing, api.Tracing) {
			return false
		}
		if !cmp.Equal([]string(spec.VhostSettings.Tags), []string(api.Tags), cmpopts.EquateEmpty()) {
			return false
		}
	}
	return true
}
