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

package binding

import (
	"context"
	"fmt"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/google/go-cmp/cmp"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	"github.com/pnowy/provider-rabbitmq/apis/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/features"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errNotBinding   = "managed resource is not a Binding custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"
	errNewClient    = "cannot create new Service"
	errGetFailed    = "cannot get RabbitMq Binding"
	errCreateFailed = "cannot create new Binding"
	errDeleteFailed = "cannot delete Binding"
	errUpdateFailed = "cannot update Binding"
)

// Setup adds a controller that reconciles Binding managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.BindingGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.BindingGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newServiceFn: rabbitmqclient.NewClient}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Binding{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(creds []byte) (*rabbitmqclient.RabbitMqService, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Binding)
	if !ok {
		return nil, errors.New(errNotBinding)
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
	service *rabbitmqclient.RabbitMqService
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Binding)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotBinding)
	}

	// These fmt statements should be removed in the real implementation.
	fmt.Printf("Observing binding: %+v\n", cr.Name)

	var bindings []rabbithole.BindingInfo
	var err error

	if cr.Status.AtProvider.Vhost != "" {
		bindings, err = listBindings(cr.Status.AtProvider.Vhost, cr.Status.AtProvider.Source, cr.Status.AtProvider.Destination, cr.Status.AtProvider.DestinationType, c.service)
	}

	if len(bindings) == 0 {
		bindings, err = listBindings(cr.Spec.ForProvider.Vhost, cr.Spec.ForProvider.Source, cr.Spec.ForProvider.Destination, cr.Spec.ForProvider.DestinationType, c.service)
	}

	if err != nil {
		if rabbitmqclient.IsNotFoundError(err) {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}

	bindingFound := false
	isResourceLateInitialized := false
	isBindingUptoDate := false
	var binding *rabbithole.BindingInfo

	if cr.Status.AtProvider.Vhost != "" {
		binding = getBindingFromObservation(bindings, &cr.Status.AtProvider)
	}

	if binding == nil {
		binding = getBinding(bindings, cr)
	}

	if binding != nil {
		bindingFound = true
		current := cr.Spec.ForProvider.DeepCopy()
		isResourceLateInitialized = !cmp.Equal(current, &cr.Spec.ForProvider)
		cr.Status.AtProvider = GenerateBindingObservation(binding)
		cr.Status.SetConditions(xpv1.Available())
		isBindingUptoDate = isUpToDate(&cr.Spec.ForProvider, binding)
	}

	fmt.Printf("Reconciling binding: %v (IsUpToDate: %v, LateInitializeBinding: %v)\n", cr.Name, isBindingUptoDate, isResourceLateInitialized)

	if !bindingFound {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	return managed.ExternalObservation{
		// Return false when the external resource does not exist. This lets
		// the managed resource reconciler know that it needs to call Create to
		// (re)create the resource, or that it has successfully been deleted.
		ResourceExists: true,

		// Return false when the external resource exists, but it not up to date
		// with the desired managed resource state. This lets the managed
		// resource reconciler know that it needs to call Update.
		ResourceUpToDate: isBindingUptoDate,

		ResourceLateInitialized: isResourceLateInitialized,
		// Return any details that may be required to connect to the external
		// resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Binding)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotBinding)
	}

	fmt.Printf("Creating binding: %+v\n", cr.Name)
	bindingInfo := GenerateBindingInfo(&cr.Spec.ForProvider)
	resp, err := c.service.Rmqc.DeclareBinding(cr.Spec.ForProvider.Vhost, bindingInfo)

	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		fmt.Printf("Error closing response body: %v\n", err)
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Binding)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotBinding)
	}

	fmt.Printf("Updating binding: %+v\n", cr.Name)

	bindings, err := listBindings(cr.Status.AtProvider.Vhost, cr.Status.AtProvider.Source, cr.Status.AtProvider.Destination, cr.Status.AtProvider.DestinationType, c.service)

	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateFailed)
	}

	// Current Binding
	binding := getBindingFromObservation(bindings, &cr.Status.AtProvider)

	// Binding not found, skipping removal
	if binding != nil {
		// Updates require removal and creation
		resp, err := c.service.Rmqc.DeleteBinding(binding.Vhost, *binding)

		if err != nil {
			fmt.Printf("(Update) Error deleting binding: %+v\n", err)
			return managed.ExternalUpdate{}, errors.Wrap(err, errDeleteFailed)
		}

		if err := resp.Body.Close(); err != nil {
			fmt.Printf("Error closing response body: %v\n", err)
		}

	}

	// Creating new binding with new config
	bindingInfo := GenerateBindingInfo(&cr.Spec.ForProvider)
	resp, err := c.service.Rmqc.DeclareBinding(cr.Spec.ForProvider.Vhost, bindingInfo)

	if err != nil {
		fmt.Printf("(Update) Error creating binding: %+v\n", err)
		return managed.ExternalUpdate{}, errors.Wrap(err, errCreateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		fmt.Printf("Error closing response body: %v\n", err)
	}

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Binding)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotBinding)
	}
	fmt.Printf("Deleting binding: %+v\n", cr.Name)

	bindingInfo := GenerateBindingInfo(&cr.Spec.ForProvider)

	resp, err := c.service.Rmqc.DeleteBinding(cr.Spec.ForProvider.Vhost, bindingInfo)

	if err != nil {
		fmt.Printf("Error deleting binding: %+v\n", err)
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteFailed)
	}
	if err := resp.Body.Close(); err != nil {
		fmt.Printf("Error closing response body: %v\n", err)
	}

	return managed.ExternalDelete{}, nil
}
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func GenerateBindingInfo(binding *v1alpha1.BindingParameters) rabbithole.BindingInfo {
	bidingInfo := rabbithole.BindingInfo{
		Source:          binding.Source,
		Destination:     binding.Destination,
		DestinationType: binding.DestinationType,
		RoutingKey:      binding.RoutingKey,
		// RabbitMQ API: /api/bindings/{vhost}/e/{source}/{destination_type}/{destination}/routing_key
		// Library Delete method: /api/bindings/{vhost}/e/{source}/{destination_type}/{destination}/{properties_keys}
		// Then: PropertiesKey = RoutingKey
		PropertiesKey: binding.RoutingKey,
	}
	return bidingInfo
}

func GenerateBindingObservation(api *rabbithole.BindingInfo) v1alpha1.BindingObservation {
	if api == nil {
		return v1alpha1.BindingObservation{}
	}
	binding := v1alpha1.BindingObservation{
		Source:          api.Source,
		Vhost:           api.Vhost,
		Destination:     api.Destination,
		DestinationType: api.DestinationType,
		RoutingKey:      api.RoutingKey,
	}

	return binding
}

func isUpToDate(spec *v1alpha1.BindingParameters, api *rabbithole.BindingInfo) bool { //nolint:gocyclo

	if spec.Source != api.Source {
		return false
	}
	if spec.Vhost != api.Vhost {
		return false
	}
	if spec.Destination != api.Destination {
		return false
	}
	if spec.DestinationType != api.DestinationType {
		return false
	}
	if spec.RoutingKey != api.RoutingKey {
		return false
	}
	return true
}

func listBindings(vhost string, source string, destination string, destinationType string, client *rabbitmqclient.RabbitMqService) (bindings []rabbithole.BindingInfo, err error) {
	switch destinationType {
	case "queue":
		bindings, err = client.Rmqc.ListQueueBindingsBetween(vhost, source, destination)
	case "exchange":
		bindings, err = client.Rmqc.ListExchangeBindingsBetween(vhost, source, destination)
	default:
		bindings, err = client.Rmqc.ListBindingsIn(vhost)

	}
	return bindings, err
}

func getBinding(bindings []rabbithole.BindingInfo, cr *v1alpha1.Binding) *rabbithole.BindingInfo {
	for _, binding := range bindings {
		if binding.Source == cr.Spec.ForProvider.Source &&
			binding.Destination == cr.Spec.ForProvider.Destination &&
			binding.DestinationType == cr.Spec.ForProvider.DestinationType &&
			binding.RoutingKey == cr.Spec.ForProvider.RoutingKey {
			return &binding
		}
	}
	return nil
}

func getBindingFromObservation(bindings []rabbithole.BindingInfo, ob *v1alpha1.BindingObservation) *rabbithole.BindingInfo {
	for _, binding := range bindings {
		if binding.Source == ob.Source &&
			binding.Destination == ob.Destination &&
			binding.DestinationType == ob.DestinationType &&
			binding.RoutingKey == ob.RoutingKey {
			return &binding
		}
	}
	return nil
}
