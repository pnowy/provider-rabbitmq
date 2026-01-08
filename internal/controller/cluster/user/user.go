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

package user

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"net/http"

	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	"github.com/pnowy/provider-rabbitmq/apis/cluster/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/cluster/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqmeta"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
)

const (
	errNotUser      = "managed resource is not a User custom resource"
	errGetFailed    = "cannot get RabbitMq User"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"
	errCreateFailed = "cannot create RabbitMq user"
	errUpdateFailed = "cannot update RabbitMq user"
	errDeleteFailed = "cannot delete RabbitMq user"

	errNewClient = "cannot create new Service"
)

func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup User controller"))
		}
	}, v1alpha1.UserGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles User managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.UserGroupKind)

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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.UserList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.UserList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.UserGroupVersionKind), opts...)

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
	cr, ok := mg.(*v1alpha1.User)
	if !ok {
		return nil, errors.New(errNotUser)
	}
	var cd apisv1alpha1.ProviderCredentials

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}
	cd = pc.Spec.Credentials

	data, err := resource.CommonCredentialExtractor(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	svc, err := c.newServiceFn(data)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{kube: c.kube, service: svc, log: c.logger}, nil
}

type external struct {
	kube    client.Client
	service *rabbitmqclient.RabbitMqService
	log     logging.Logger
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.User)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotUser)
	}
	name := getExternalName(cr)
	apiUser, err := c.service.Rmqc.GetUser(name)
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
	lateInitialize(&cr.Spec.ForProvider, apiUser)
	isResourceLateInitialized := !cmp.Equal(current, &cr.Spec.ForProvider)

	cr.Status.AtProvider = generateUserObservation(apiUser)
	cr.Status.SetConditions(xpv1.Available())

	password, err := c.resolveUserPassword(ctx, cr.Spec.ForProvider.UserSettings)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot resolve password")
	}
	isUpToDate := isUpToDate(password, &cr.Spec.ForProvider, apiUser)
	c.log.Info("Reconciling user", "username", name, "upToDate", isUpToDate, "lateInitialized", isResourceLateInitialized)

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        isUpToDate,
		ResourceLateInitialized: isResourceLateInitialized,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.User)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotUser)
	}
	name := getExternalName(cr)

	c.log.Info("Creating user", "username", name)

	password, err := c.resolveUserPassword(ctx, cr.Spec.ForProvider.UserSettings)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot determine password")
	}

	var resp *http.Response
	if password != "" {
		resp, err = c.service.Rmqc.PutUser(name, generateApiUserSettings(&password, cr.Spec.ForProvider.UserSettings))
		if resp != nil {
			if err := resp.Body.Close(); err != nil {
				c.log.Debug("Error closing response body", "err", err)
			}
		}
	} else {
		resp, err = c.service.Rmqc.PutUserWithoutPassword(name, generateApiUserSettings(nil, cr.Spec.ForProvider.UserSettings))
		if resp != nil {
			if err := resp.Body.Close(); err != nil {
				c.log.Debug("Error closing response body", "err", err)
			}
		}
	}
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}
	rabbitmqmeta.SetCrossplaneManaged(cr, name)
	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.User)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotUser)
	}
	name := getExternalName(cr)
	c.log.Info("Updating user", "username", name)
	password, err := c.resolveUserPassword(ctx, cr.Spec.ForProvider.UserSettings)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot determine password")
	}
	var resp *http.Response
	if password != "" {
		resp, err = c.service.Rmqc.PutUser(name, generateApiUserSettings(&password, cr.Spec.ForProvider.UserSettings))
		if resp != nil {
			if err := resp.Body.Close(); err != nil {
				c.log.Debug("Error closing response body", "err", err)
			}
		}
	} else {
		resp, err = c.service.Rmqc.PutUserWithoutPassword(name, generateApiUserSettings(nil, cr.Spec.ForProvider.UserSettings))
		if resp != nil {
			if err := resp.Body.Close(); err != nil {
				c.log.Debug("Error closing response body", "err", err)
			}
		}
	}
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateFailed)
	}
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.User)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotUser)
	}
	name := getExternalName(cr)
	c.log.Info("Deleting user", "username", name)
	resp, err := c.service.Rmqc.DeleteUser(name)
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

func getExternalName(spec *v1alpha1.User) string {
	forProviderName := spec.Spec.ForProvider.Username
	if forProviderName != nil {
		return *forProviderName
	}
	return spec.Name
}

func (c *external) resolveUserPassword(ctx context.Context, spec *v1alpha1.UserSettings) (string, error) {
	// passwordless user
	if spec == nil {
		return "", nil
	}
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

func generateApiUserSettings(password *string, spec *v1alpha1.UserSettings) rabbithole.UserSettings {
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

func lateInitialize(spec *v1alpha1.UserParameters, api *rabbithole.UserInfo) {
	if api == nil {
		return
	}
	if spec.UserSettings == nil {
		spec.UserSettings = &v1alpha1.UserSettings{}
	}
	if len(api.Tags) > 0 && len(spec.UserSettings.Tags) > 0 {
		spec.UserSettings.Tags = make([]string, len(api.Tags))
		copy(spec.UserSettings.Tags, api.Tags)
	}
}

func generateUserObservation(apiUser *rabbithole.UserInfo) v1alpha1.UserObservation {
	if apiUser == nil {
		return v1alpha1.UserObservation{}
	}
	user := v1alpha1.UserObservation{
		Username:         apiUser.Name,
		HashingAlgorithm: string(apiUser.HashingAlgorithm),
	}
	if len(apiUser.Tags) > 0 {
		user.Tags = make([]string, len(apiUser.Tags))
		copy(user.Tags, apiUser.Tags)
	}
	return user
}

func isUpToDate(password string, spec *v1alpha1.UserParameters, api *rabbithole.UserInfo) bool { //nolint:gocyclo
	if password != "" && api.PasswordHash != "" {
		if api.HashingAlgorithm == rabbithole.HashingAlgorithmSHA256 {
			if !isPasswordUpToDateSHA256(password, api.PasswordHash) {
				return false
			}
		}
		if api.HashingAlgorithm == rabbithole.HashingAlgorithmSHA512 {
			if !isPasswordUpToDateSHA512(password, api.PasswordHash) {
				return false
			}
		}
	}
	if spec.UserSettings != nil {
		if !cmp.Equal([]string(spec.UserSettings.Tags), []string(api.Tags), cmpopts.EquateEmpty()) {
			return false
		}
	}
	return true
}

func isPasswordUpToDateSHA256(password, storedBase64Hash string) bool {
	decoded, err := base64.StdEncoding.DecodeString(storedBase64Hash)
	if err != nil {
		return false
	}

	// Extract salt (first 4 characters)
	if len(decoded) < 4 {
		return false
	}
	salt := string(decoded[:4])
	expectedHash := sha256.Sum256([]byte(salt + password))

	// Compare expected hash with stored hash
	return bytes.Equal(decoded[4:], expectedHash[:])
}

func isPasswordUpToDateSHA512(password, storedBase64Hash string) bool {
	decoded, err := base64.StdEncoding.DecodeString(storedBase64Hash)
	if err != nil {
		return false
	}

	// Extract salt (first 4 characters)
	if len(decoded) < 4 {
		return false
	}
	salt := string(decoded[:4])
	expectedHash := sha512.Sum512([]byte(salt + password))

	return bytes.Equal(decoded[4:], expectedHash[:])
}
