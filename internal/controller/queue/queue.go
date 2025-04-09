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

package queue

import (
	"context"
	"fmt"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/google/go-cmp/cmp"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"

	"github.com/pkg/errors"
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
)

const (
	errNotQueue     = "managed resource is not a Queue custom resource"
	errGetFailed    = "cannot get RabbitMq queue"
	errCreateFailed = "cannot create RabbitMq queue"
	errDeleteFailed = "cannot delete RabbitMq queue"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errNewClient = "cannot create new Service"
)

// Setup adds a controller that reconciles Queue managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.QueueGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.QueueGroupVersionKind),
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
		For(&v1alpha1.Queue{}).
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
	cr, ok := mg.(*v1alpha1.Queue)
	if !ok {
		return nil, errors.New(errNotQueue)
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

type external struct {
	service *rabbitmqclient.RabbitMqService
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Queue)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotQueue)
	}

	queueName := getQueueName(cr)
	fmt.Printf("Observing: %+v\n", queueName)
	apiQueue, err := c.service.Rmqc.GetQueue(cr.Spec.ForProvider.Vhost, queueName)
	if err != nil {
		if rabbitmqclient.IsNotFoundError(err) {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}
	current := cr.Spec.ForProvider.DeepCopy()
	lateInitialize(&cr.Spec.ForProvider, apiQueue)
	isResourceLateInitialized := !cmp.Equal(current, &cr.Spec.ForProvider)

	cr.Status.AtProvider = generateQueueObservation(apiQueue)
	cr.Status.SetConditions(xpv1.Available())

	isExchangeUptoDate := isUpToDate(&cr.Spec.ForProvider, apiQueue)

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        isExchangeUptoDate,
		ResourceLateInitialized: isResourceLateInitialized,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Queue)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotQueue)
	}

	queueName := getQueueName(cr)
	fmt.Printf("Creating queue: %+v\n", queueName)

	resp, err := c.service.Rmqc.DeclareQueue(cr.Spec.ForProvider.Vhost, queueName, generateQueueSettings(cr.Spec.ForProvider.QueueSettings))

	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		fmt.Printf("Error closing response body: %v\n", err)
	}

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Queue)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotQueue)
	}

	fmt.Printf("Updating: %+v", cr)

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Queue)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotQueue)
	}

	queueName := getQueueName(cr)
	fmt.Printf("Deleting queue: %+v\n", queueName)
	resp, err := c.service.Rmqc.DeleteQueue(cr.Spec.ForProvider.Vhost, queueName)

	if err != nil {
		fmt.Printf("Error deleting queue: %+v\n", err)
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

func getQueueName(spec *v1alpha1.Queue) string {
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
	return settings
}

func lateInitialize(spec *v1alpha1.QueueParameters, api *rabbithole.DetailedQueueInfo) {
	if api == nil {
		return
	}
	if spec.QueueSettings == nil {
		spec.QueueSettings = &v1alpha1.QueueSettings{}
	}
	// TODO late init
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
	}
	return true
}
