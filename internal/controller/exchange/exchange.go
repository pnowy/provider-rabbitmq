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

package exchange

import (
	"context"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/google/go-cmp/cmp"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/pnowy/provider-rabbitmq/apis/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/features"
)

const (
	errNotExchange        = "managed resource is not a Exchange custom resource"
	errTrackPCUsage       = "cannot track ProviderConfig usage"
	errGetPC              = "cannot get ProviderConfig"
	errGetCreds           = "cannot get credentials"
	errGetFailed          = "cannot get RabbitMq Exchange"
	errNewClient          = "cannot create new Service"
	errCreateFailed       = "cannot create new Exchange"
	errDeleteFailed       = "cannot delete Exchange"
	errUpdateFailed       = "cannot update Exchange"
	EXCHANGE_DEFAULT_TYPE = "fanout"
)

// Setup adds a controller that reconciles Exchange managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.ExchangeGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.ExchangeGroupVersionKind),
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
		For(&v1alpha1.Exchange{}).
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
	cr, ok := mg.(*v1alpha1.Exchange)
	if !ok {
		return nil, errors.New(errNotExchange)
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

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	service *rabbitmqclient.RabbitMqService
	log     logging.Logger
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Exchange)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotExchange)
	}

	exchangeName := getExchangeName(cr)
	c.log.Info("Observing exchange", "exchange", exchangeName)

	apiExchange, err := c.service.Rmqc.GetExchange(cr.Spec.ForProvider.Vhost, exchangeName)

	if err != nil {
		if rabbitmqclient.IsNotFoundError(err) {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}
	current := cr.Spec.ForProvider.DeepCopy()
	lateInitialize(&cr.Spec.ForProvider, apiExchange)
	isResourceLateInitialized := !cmp.Equal(current, &cr.Spec.ForProvider)
	cr.Status.AtProvider = generateExchangeObservation(apiExchange)
	cr.Status.SetConditions(xpv1.Available())

	isExchangeUptoDate := isUpToDate(&cr.Spec.ForProvider, apiExchange)

	c.log.Debug("Reconciling exchange", "exchange", exchangeName, "current", current, "isUpToDate", isExchangeUptoDate)

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        isExchangeUptoDate,
		ResourceLateInitialized: isResourceLateInitialized,
		ConnectionDetails:       managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Exchange)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotExchange)
	}
	exchangeName := getExchangeName(cr)
	c.log.Info("Creating exchange", "exchange", exchangeName)

	resp, err := c.service.Rmqc.DeclareExchange(cr.Spec.ForProvider.Vhost, exchangeName, generateExchangeOptions(cr.Spec.ForProvider.ExchangeSettings))

	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		c.log.Debug(err.Error(), "failed to close response body")
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Exchange)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotExchange)
	}

	exchangeName := getExchangeName(cr)
	c.log.Info("Updating exchange", "exchange", exchangeName)

	resp, err := c.service.Rmqc.DeclareExchange(cr.Spec.ForProvider.Vhost, exchangeName, generateExchangeOptions(cr.Spec.ForProvider.ExchangeSettings))

	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		c.log.Debug(err.Error(), "failed to close response body")
	}

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Exchange)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotExchange)
	}

	exchangeName := getExchangeName(cr)
	c.log.Info("Deleting exchange", "exchange", exchangeName)

	resp, err := c.service.Rmqc.DeleteExchange(cr.Spec.ForProvider.Vhost, exchangeName)

	if err != nil {
		c.log.Debug(err.Error(), "failed to delete exchange", "exchange", exchangeName)
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteFailed)
	}
	if err := resp.Body.Close(); err != nil {
		c.log.Debug(err.Error(), "failed to close response body")
	}
	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func lateInitialize(spec *v1alpha1.ExchangeParameters, api *rabbithole.DetailedExchangeInfo) {
	if api == nil {
		return
	}
	if spec.ExchangeSettings == nil {
		spec.ExchangeSettings = &v1alpha1.ExchangeSettings{}
	}
}

func generateExchangeObservation(api *rabbithole.DetailedExchangeInfo) v1alpha1.ExchangeObservation {
	if api == nil {
		return v1alpha1.ExchangeObservation{}
	}
	exchange := v1alpha1.ExchangeObservation{
		Name:       api.Name,
		Vhost:      api.Vhost,
		Type:       api.Type,
		Durable:    api.Durable,
		AutoDelete: api.AutoDelete,
	}

	return exchange
}

func generateExchangeOptions(spec *v1alpha1.ExchangeSettings) rabbithole.ExchangeSettings {
	if spec == nil {
		settings := rabbithole.ExchangeSettings{}
		// Default value (Type is required in rabbitMq api)
		settings.Type = EXCHANGE_DEFAULT_TYPE
		return settings
	}
	settings := rabbithole.ExchangeSettings{}
	if spec.Type != nil {
		settings.Type = *spec.Type
	} else {
		// Default value (Type is required in rabbitMq api)
		settings.Type = EXCHANGE_DEFAULT_TYPE
	}

	if spec.Durable != nil {
		settings.Durable = *spec.Durable
	}

	if spec.AutoDelete != nil {
		settings.AutoDelete = *spec.AutoDelete
	}
	return settings
}

func isUpToDate(spec *v1alpha1.ExchangeParameters, api *rabbithole.DetailedExchangeInfo) bool { //nolint:gocyclo
	if spec.ExchangeSettings != nil {
		if !rabbitmqclient.IsStringPtrEqualToString(spec.ExchangeSettings.Type, api.Type) {
			return false
		}
		if !rabbitmqclient.IsBoolPtrEqualToBool(spec.ExchangeSettings.Durable, api.Durable) {
			return false
		}
		if !rabbitmqclient.IsBoolPtrEqualToBool(spec.ExchangeSettings.AutoDelete, api.AutoDelete) {
			return false
		}
	}
	return true
}

func getExchangeName(spec *v1alpha1.Exchange) string {
	forProviderName := spec.Spec.ForProvider.Name
	if forProviderName != nil {
		return *forProviderName
	}
	return spec.Name
}
