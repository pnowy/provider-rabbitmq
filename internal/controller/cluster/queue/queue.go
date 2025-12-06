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

package queue

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	"github.com/pnowy/provider-rabbitmq/apis/cluster/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/cluster/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqmeta"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/google/go-cmp/cmp"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
)

const (
	errNotQueue     = "managed resource is not a Queue custom resource"
	errGetFailed    = "cannot get RabbitMq queue"
	errCreateFailed = "cannot create RabbitMq queue"
	errDeleteFailed = "cannot delete RabbitMq queue"
	errUpdateFailed = "cannot update RabbitMq queue"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errNewClient = "cannot create new Service"
)

// SetupGated adds a controller that reconciles Queue managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup Queue controller"))
		}
	}, v1alpha1.QueueGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles Queue managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.QueueGroupKind)
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.QueueList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.Queue")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.QueueGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Queue{}).
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
	cr, ok := mg.(*v1alpha1.Queue)
	if !ok {
		return nil, errors.New(errNotQueue)
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
	cr, ok := mg.(*v1alpha1.Queue)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotQueue)
	}

	name := getExternalName(cr)
	apiQueue, err := c.service.Rmqc.GetQueue(cr.Spec.ForProvider.Vhost, name)
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
	lateInitialize(&cr.Spec.ForProvider, apiQueue)
	isResourceLateInitialized := !cmp.Equal(current, &cr.Spec.ForProvider)

	cr.Status.AtProvider = generateQueueObservation(apiQueue)
	cr.Status.SetConditions(xpv1.Available())

	isUptoDate := isUpToDate(&cr.Spec.ForProvider, apiQueue)

	c.log.Info("Reconciling queue", "queue", name, "upToDate", isUptoDate, "lateInitialized", isResourceLateInitialized)

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        isUptoDate,
		ResourceLateInitialized: isResourceLateInitialized,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Queue)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotQueue)
	}

	name := getExternalName(cr)
	c.log.Info("Creating queue", "queue", name)

	resp, err := c.service.Rmqc.DeclareQueue(cr.Spec.ForProvider.Vhost, name, generateQueueSettings(cr.Spec.ForProvider.QueueSettings))

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
	cr, ok := mg.(*v1alpha1.Queue)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotQueue)
	}

	name := getExternalName(cr)
	c.log.Info("Updating queue", "queue", name)

	resp, err := c.service.Rmqc.DeclareQueue(cr.Spec.ForProvider.Vhost, name, generateQueueSettings(cr.Spec.ForProvider.QueueSettings))

	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		c.log.Debug("Error closing response body", "err", err)
	}

	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Queue)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotQueue)
	}

	name := getExternalName(cr)
	c.log.Info("Deleting queue", "queue", name)
	resp, err := c.service.Rmqc.DeleteQueue(cr.Spec.ForProvider.Vhost, name)

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

func getExternalName(spec *v1alpha1.Queue) string {
	forProviderName := spec.Spec.ForProvider.Name
	if forProviderName != nil {
		return *forProviderName
	}
	return spec.Name
}

func generateQueueSettings(spec *v1alpha1.QueueSettings) rabbithole.QueueSettings {
	if spec == nil {
		return rabbithole.QueueSettings{}
	}
	settings := rabbithole.QueueSettings{}
	if spec.Type != nil {
		settings.Type = *spec.Type
	}
	if spec.AutoDelete != nil {
		settings.AutoDelete = *spec.AutoDelete
	}
	if spec.Durable != nil {
		settings.Durable = *spec.Durable
	}
	if spec.Arguments != nil {
		settings.Arguments = rabbitmqclient.ConvertStringMapToInterfaceMap(spec.Arguments)
	}
	return settings
}

func lateInitialize(spec *v1alpha1.QueueParameters, api *rabbithole.DetailedQueueInfo) {
	if api == nil {
		return
	}
	if spec.QueueSettings == nil {
		spec.QueueSettings = &v1alpha1.QueueSettings{}
	}
	if spec.QueueSettings.Type == nil {
		spec.QueueSettings.Type = &api.Type
	}
	if spec.QueueSettings.AutoDelete == nil {
		autoDelete := bool(api.AutoDelete)
		spec.QueueSettings.AutoDelete = &autoDelete
	}
	if spec.QueueSettings.Durable == nil {
		spec.QueueSettings.Durable = &api.Durable
	}
	if spec.QueueSettings.Arguments == nil {
		spec.QueueSettings.Arguments = rabbitmqclient.ConvertInterfaceMapToStringMap(api.Arguments)
	}
}

func generateQueueObservation(api *rabbithole.DetailedQueueInfo) v1alpha1.QueueObservation {
	if api == nil {
		return v1alpha1.QueueObservation{}
	}
	observation := v1alpha1.QueueObservation{
		Name:       api.Name,
		Vhost:      api.Vhost,
		Type:       api.Type,
		Durable:    api.Durable,
		AutoDelete: bool(api.AutoDelete),
		Arguments:  rabbitmqclient.ConvertInterfaceMapToStringMap(api.Arguments),
	}

	return observation
}

func isUpToDate(spec *v1alpha1.QueueParameters, api *rabbithole.DetailedQueueInfo) bool { //nolint:gocyclo
	if spec.QueueSettings != nil {
		if !rabbitmqclient.IsStringPtrEqualToString(spec.QueueSettings.Type, api.Type) {
			return false
		}
		if !rabbitmqclient.IsBoolPtrEqualToBool(spec.QueueSettings.Durable, api.Durable) {
			return false
		}
		if !rabbitmqclient.IsBoolPtrEqualToBool(spec.QueueSettings.AutoDelete, bool(api.AutoDelete)) {
			return false
		}
		argumentsUpToDate, _ := rabbitmqclient.MapsEqualJSON(spec.QueueSettings.Arguments, rabbitmqclient.ConvertInterfaceMapToStringMap(api.Arguments))
		return argumentsUpToDate
	}
	return true
}
