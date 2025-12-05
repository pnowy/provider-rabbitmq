/*
Copyright 2025 The Crossplane Authors.

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
	"net/url"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/pnowy/provider-rabbitmq/apis/cluster/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/cluster/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqmeta"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	"github.com/google/go-cmp/cmp"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errNotBinding                     = "managed resource is not a Binding custom resource"
	errGetPC                          = "cannot get ProviderConfig"
	errGetCreds                       = "cannot get credentials"
	errNewClient                      = "cannot create new Service"
	errGetFailed                      = "cannot get RabbitMq Binding"
	errCreateFailed                   = "cannot create new Binding"
	errDeleteFailed                   = "cannot delete Binding"
	errorUpdateNotSupported           = "Binding cannot be updated. Please, Try to recreate it"
	propertiesKeyExpectedPartsCounter = 5
)

func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup Binding controller"))
		}
	}, v1alpha1.BindingGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles Binding managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.BindingGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithExternalConnector(&connector{
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newServiceFn: rabbitmqclient.NewClient,
			logger:       o.Logger.WithValues("controller", name)}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	}

	if o.Features.Enabled(feature.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}

	if o.Features.Enabled(feature.EnableAlphaChangeLogs) {
		opts = append(opts, managed.WithChangeLogger(o.ChangeLogOptions.ChangeLogger))
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.BindingList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.Binding")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.BindingGroupVersionKind), opts...)

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
	usage        *resource.ProviderConfigUsageTracker
	newServiceFn func(creds []byte) (*rabbitmqclient.RabbitMqService, error)
	logger       logging.Logger
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
	cr, ok := mg.(*v1alpha1.Binding)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotBinding)
	}

	name := cr.Name
	bindings, err := listBindings(cr.Spec.ForProvider.Vhost, cr.Spec.ForProvider.Source, cr.Spec.ForProvider.Destination, cr.Spec.ForProvider.DestinationType, c.service)

	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}

	if len(bindings) == 0 {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	if rabbitmqmeta.IsNotCrossplaneManaged(cr) {
		return managed.ExternalObservation{}, rabbitmqmeta.NewNotCrossplaneManagedError(name)
	}

	bindingFound := false
	isResourceLateInitialized := false
	isBindingUptoDate := false

	binding := getBinding(bindings, cr)

	if binding != nil {
		bindingFound = true
		current := cr.Spec.ForProvider.DeepCopy()
		isResourceLateInitialized = !cmp.Equal(current, &cr.Spec.ForProvider)
		cr.Status.AtProvider = generateBindingObservation(binding)
		cr.Status.SetConditions(xpv1.Available())
		isBindingUptoDate = isUpToDate(&cr.Spec.ForProvider, binding)
	}

	c.log.Info("Reconciling binding", "binding", name, "upToDate", isBindingUptoDate, "lateInitialized", isResourceLateInitialized)

	if !bindingFound {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        isBindingUptoDate,
		ResourceLateInitialized: isResourceLateInitialized,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Binding)

	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotBinding)
	}

	c.log.Info("Creating binding", "binding", cr.Name)
	bindingInfo := generateBindingInfo(&cr.Spec.ForProvider)
	resp, err := c.service.Rmqc.DeclareBinding(cr.Spec.ForProvider.Vhost, bindingInfo)

	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}

	// ID returned by RabbitMQ server
	location := strings.Split(resp.Header.Get("Location"), "/")
	propertiesKey, err := url.PathUnescape(location[len(location)-1])

	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}

	name := getExternalName(&cr.Spec.ForProvider, propertiesKey)
	rabbitmqmeta.SetCrossplaneManaged(cr, name)

	if err := resp.Body.Close(); err != nil {
		c.log.Debug("Error closing response body", "err", err)
	}

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	return managed.ExternalUpdate{}, errors.New(errorUpdateNotSupported)
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Binding)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotBinding)
	}
	c.log.Info("Deleting binding", "binding", cr.Name)

	bindingInfo := generateBindingInfo(&cr.Spec.ForProvider)
	// Getting Properties Keys from Status
	bindingInfo.PropertiesKey = cr.Status.AtProvider.PropertiesKey
	resp, err := c.service.Rmqc.DeleteBinding(cr.Spec.ForProvider.Vhost, bindingInfo)

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

func generateBindingInfo(binding *v1alpha1.BindingParameters) rabbithole.BindingInfo {
	arguments := rabbitmqclient.ConvertStringMapToInterfaceMap(binding.Arguments)
	bidingInfo := rabbithole.BindingInfo{
		Source:          binding.Source,
		Destination:     binding.Destination,
		DestinationType: binding.DestinationType,
		RoutingKey:      binding.RoutingKey,
		Arguments:       arguments,
	}
	return bidingInfo
}

func generateBindingObservation(api *rabbithole.BindingInfo) v1alpha1.BindingObservation {
	if api == nil {
		return v1alpha1.BindingObservation{}
	}
	binding := v1alpha1.BindingObservation{
		Source:          api.Source,
		Vhost:           api.Vhost,
		Destination:     api.Destination,
		DestinationType: api.DestinationType,
		RoutingKey:      api.RoutingKey,
		PropertiesKey:   api.PropertiesKey,
		Arguments:       rabbitmqclient.ConvertInterfaceMapToStringMap(api.Arguments),
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
	areArgumentsUpToDate, _ := rabbitmqclient.MapsEqualJSON(spec.Arguments, rabbitmqclient.ConvertInterfaceMapToStringMap(api.Arguments))

	return areArgumentsUpToDate
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

	// ID Expected = vhost/source/destination_type/destination/propertiesKey
	id := strings.Split(cr.Annotations["crossplane.io/external-name"], "/")

	// Checking if external name is updated with id
	if len(id) < propertiesKeyExpectedPartsCounter {
		return nil
	}

	propertiesKey := id[4]
	for _, binding := range bindings {
		if binding.Source == cr.Spec.ForProvider.Source &&
			binding.Destination == cr.Spec.ForProvider.Destination &&
			binding.DestinationType == cr.Spec.ForProvider.DestinationType &&
			binding.PropertiesKey == propertiesKey {
			return &binding
		}
	}
	return nil
}

func getExternalName(binding *v1alpha1.BindingParameters, propertiesKey string) string {
	return binding.Vhost + "/" + binding.Source + "/" + binding.DestinationType + "/" + binding.Destination + "/" + propertiesKey
}
