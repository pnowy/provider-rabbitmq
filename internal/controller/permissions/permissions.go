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

package permissions

import (
	"context"
	"fmt"
	"github.com/crossplane/crossplane-runtime/pkg/logging"

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
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pnowy/provider-rabbitmq/apis/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/features"
)

const (
	errNotPermissions = "managed resource is not a Permissions custom resource"
	errTrackPCUsage   = "cannot track ProviderConfig usage"
	errGetPC          = "cannot get ProviderConfig"
	errGetCreds       = "cannot get credentials"
	errGetFailed      = "cannot get RabbitMq User Permissions"
	errNewClient      = "cannot create new Service"
	errCreateFailed   = "cannot create new User Permissions"
	errDeleteFailed   = "cannot delete User Permissions"
	errUpdateFailed   = "cannot update User Permissions"
)

// Setup adds a controller that reconciles Permissions managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.PermissionsGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.PermissionsGroupVersionKind),
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
		For(&v1alpha1.Permissions{}).
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
	cr, ok := mg.(*v1alpha1.Permissions)
	c.logger.Debug("Creating External client")

	if !ok {
		return nil, errors.New(errNotPermissions)
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
	cr, ok := mg.(*v1alpha1.Permissions)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotPermissions)
	}

	c.log.Info("Observing user permissions", "permissions", getPermissionsExternalName(&cr.Spec.ForProvider))

	userPerms, err := c.service.Rmqc.GetPermissionsIn(cr.Spec.ForProvider.Vhost, cr.Spec.ForProvider.User)

	if err != nil {
		if rabbitmqclient.IsNotFoundError(err) {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}

	current := cr.Spec.ForProvider.DeepCopy()
	lateInitialize(&cr.Spec.ForProvider, &userPerms)
	isResourceLateInitialized := !cmp.Equal(current, &cr.Spec.ForProvider)
	cr.Status.AtProvider = GenerateExchangeObservation(&userPerms)
	cr.Status.SetConditions(xpv1.Available())
	isPermissionsUptoDate := isUpToDate(&cr.Spec.ForProvider, &userPerms)

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        isPermissionsUptoDate,
		ResourceLateInitialized: isResourceLateInitialized,
		ConnectionDetails:       managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Permissions)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotPermissions)
	}

	c.log.Info("Creating user permissions", "permissions", getPermissionsExternalName(&cr.Spec.ForProvider))
	userPerms := GeneratePermissionSettings(cr.Spec.ForProvider.PermissionSettings)
	resp, err := c.service.Rmqc.UpdatePermissionsIn(cr.Spec.ForProvider.Vhost, cr.Spec.ForProvider.User, userPerms)

	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		fmt.Printf("Error closing response body: %v\n", err)
	}

	c.log.Debug("User permissions created in RabbitMQ server", "permissions", getPermissionsExternalName(&cr.Spec.ForProvider))

	// Storing ID in external name
	meta.SetExternalName(cr, getPermissionsExternalName(&cr.Spec.ForProvider))
	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Permissions)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotPermissions)
	}

	c.log.Info("Updating user permissions", "permissions", getPermissionsExternalName(&cr.Spec.ForProvider))

	userPerms := GeneratePermissionSettings(cr.Spec.ForProvider.PermissionSettings)
	resp, err := c.service.Rmqc.UpdatePermissionsIn(cr.Spec.ForProvider.Vhost, cr.Spec.ForProvider.User, userPerms)

	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateFailed)
	}
	if err := resp.Body.Close(); err != nil {
		fmt.Printf("Error closing response body: %v\n", err)
	}

	c.log.Debug("User permissions updated in RabbitMQ server", "permissions", getPermissionsExternalName(&cr.Spec.ForProvider))

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Permissions)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotPermissions)
	}

	c.log.Info("Deleting user permissions", "permissions", getPermissionsExternalName(&cr.Spec.ForProvider))

	resp, err := c.service.Rmqc.ClearPermissionsIn(cr.Spec.ForProvider.Vhost, cr.Spec.ForProvider.User)

	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteFailed)
	}
	if err := resp.Body.Close(); err != nil {
		fmt.Printf("Error closing response body: %v\n", err)
	}

	c.log.Debug("User permissions deleted in RabbitMQ server", "permissions", getPermissionsExternalName(&cr.Spec.ForProvider))

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func lateInitialize(spec *v1alpha1.PermissionsParameters, api *rabbithole.PermissionInfo) {
	if api == nil {
		return
	}
	if spec.PermissionSettings == nil {
		spec.PermissionSettings = &v1alpha1.PermissionSettings{}
	}
}

func GenerateExchangeObservation(api *rabbithole.PermissionInfo) v1alpha1.PermissionsObservation {
	if api == nil {
		return v1alpha1.PermissionsObservation{}
	}
	UserPerms := v1alpha1.PermissionsObservation{
		User:      api.User,
		Vhost:     api.Vhost,
		Configure: api.Configure,
		Write:     api.Write,
		Read:      api.Read,
	}

	return UserPerms
}

func isUpToDate(spec *v1alpha1.PermissionsParameters, api *rabbithole.PermissionInfo) bool { //nolint:gocyclo
	// Vhost and User cannot be modified (immutable)
	if !rabbitmqclient.IsStringPtrEqualToString(&spec.PermissionSettings.Configure, api.Configure) {
		return false
	}
	if !rabbitmqclient.IsStringPtrEqualToString(&spec.PermissionSettings.Write, api.Write) {
		return false
	}
	if !rabbitmqclient.IsStringPtrEqualToString(&spec.PermissionSettings.Read, api.Read) {
		return false
	}
	return true
}

func GeneratePermissionSettings(spec *v1alpha1.PermissionSettings) rabbithole.Permissions {
	userPerms := rabbithole.Permissions{}
	userPerms.Configure = spec.Configure
	userPerms.Write = spec.Write
	userPerms.Read = spec.Read
	return userPerms
}

func getPermissionsExternalName(spec *v1alpha1.PermissionsParameters) string {
	return spec.Vhost + "/" + spec.User
}
