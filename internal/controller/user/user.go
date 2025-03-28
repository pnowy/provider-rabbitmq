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

package user

import (
	"context"
	"fmt"

	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
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
	errNotUser      = "managed resource is not a User custom resource"
	errGetFailed    = "cannot get RabbitMq User"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"
	errCreateFailed = "cannot create RabbitMq user"
	errDeleteFailed = "cannot delete RabbitMq user"

	errNewClient = "cannot create new Service"
)

// Setup adds a controller that reconciles User managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.UserGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.UserGroupVersionKind),
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
		For(&v1alpha1.User{}).
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
	cr, ok := mg.(*v1alpha1.User)
	if !ok {
		return nil, errors.New(errNotUser)
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

	return &external{kube: c.kube, service: svc}, nil
}

type external struct {
	kube    client.Client
	service *rabbitmqclient.RabbitMqService
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.User)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotUser)
	}
	_, err := c.service.Rmqc.GetUser(cr.Spec.ForProvider.Username)
	if err != nil {
		if rabbitmqclient.IsNotFoundError(err) {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}
	fmt.Printf("Reconciling user: %v\n", cr.Spec.ForProvider.Username)

	return managed.ExternalObservation{
		// Return false when the external resource does not exist. This lets
		// the managed resource reconciler know that it needs to call Create to
		// (re)create the resource, or that it has successfully been deleted.
		ResourceExists: true,

		// Return false when the external resource exists, but it not up to date
		// with the desired managed resource state. This lets the managed
		// resource reconciler know that it needs to call Update.
		ResourceUpToDate: true,

		// Return any details that may be required to connect to the external
		// resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.User)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotUser)
	}
	fmt.Printf("Creating user: %+v", cr.Spec.ForProvider.Username)

	// TODO check if ok to keep password this way temporary
	password, err := c.resolveUserPassword(ctx, cr.Spec.ForProvider.UserSettings)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot determine password")
	}

	resp, err := c.service.Rmqc.PutUser(cr.Spec.ForProvider.Username, generateClientUserSettings(&password, cr.Spec.ForProvider.UserSettings))
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		fmt.Printf("Error closing response body: %v\n", err)
	}

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.User)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotUser)
	}

	fmt.Printf("Updating: %+v", cr)

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.User)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotUser)
	}
	fmt.Printf("Deleting user: %+v", cr.Spec.ForProvider.Username)
	resp, err := c.service.Rmqc.DeleteUser(cr.Spec.ForProvider.Username)
	if err != nil {
		fmt.Printf("Error deleting user: %+v\n", err)
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

func (c *external) resolveUserPassword(ctx context.Context, spec *v1alpha1.UserSettings) (string, error) {
	// Direct password has precedence
	if spec.Password != nil {
		return *spec.Password, nil
	}
	// Try to resolve from secret reference
	if ref := spec.PasswordSecretRef; ref != nil {
		secret := &corev1.Secret{}
		nn := types.NamespacedName{
			Namespace: ref.Namespace,
			Name:      ref.Name,
		}
		if err := c.kube.Get(ctx, nn, secret); err != nil {
			return "", errors.Wrap(err, "cannot get password secret")
		}
		password, ok := secret.Data[ref.Key]
		if !ok {
			return "", errors.Errorf("secret %s has no key %s", ref.Name, ref.Key)
		}
		return string(password), nil
	}
	return "", nil
}

func generateClientUserSettings(password *string, spec *v1alpha1.UserSettings) rabbithole.UserSettings {
	if spec == nil {
		return rabbithole.UserSettings{}
	}
	settings := rabbithole.UserSettings{}
	if password != nil {
		settings.Password = *password
	}
	if len(spec.Tags) > 0 {
		settings.Tags = make([]string, len(spec.Tags))
		copy(settings.Tags, spec.Tags)
	}
	return settings
}
