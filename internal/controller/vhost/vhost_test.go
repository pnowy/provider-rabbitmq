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
	"net/http"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	"github.com/pnowy/provider-rabbitmq/apis/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient/fake"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqmeta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestConnect(t *testing.T) {
	type fields struct {
		kube         client.Client
		usage        *resource.ProviderConfigUsageTracker
		newServiceFn func(creds []byte) (*rabbitmqclient.RabbitMqService, error)
	}

	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   error
	}{
		"Success": {
			reason: "No error should be returned.",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						switch o := obj.(type) {
						case *apisv1alpha1.ProviderConfig:
							o.Spec.Credentials.Source = "Secret"
							o.Spec.Credentials.SecretRef = &xpv1.SecretKeySelector{
								Key: "creds",
							}
						case *corev1.Secret:
							o.Data = map[string][]byte{
								"creds": []byte("{\"APIKey\":\"foo\",\"Email\":\"foo@bar.com\"}"),
							}
						}
						return nil
					},
				},
				usage: resource.NewProviderConfigUsageTracker(&fake.MockApplicator{}, &apisv1alpha1.ProviderConfigUsage{}),
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, nil
				},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					Spec: v1alpha1.VhostSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &common.ProviderConfigReference{
								Name: "testing-provider-config",
								Kind: "ProviderConfig",
							},
						},
					},
				},
			},
			want: nil,
		},
		"Error (No ProviderConfig)": {
			reason: "Error should be returned.",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						switch o := obj.(type) {
						case *apisv1alpha1.ProviderConfig:
							return errors.New("no provider config")
						case *corev1.Secret:
							o.Data = map[string][]byte{
								"creds": []byte("{\"APIKey\":\"foo\",\"Email\":\"foo@bar.com\"}"),
							}
						}
						return nil
					},
				},
				usage: resource.NewProviderConfigUsageTracker(&fake.MockApplicator{}, &apisv1alpha1.ProviderConfigUsage{}),
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, nil
				},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					Spec: v1alpha1.VhostSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &common.ProviderConfigReference{
								Name: "testing-provider-config",
								Kind: "ProviderConfig",
							},
						},
					},
				},
			},
			want: errors.Wrap(errors.New("no provider config"), "cannot get ProviderConfig"),
		},
		"Error (No Secret)": {
			reason: "Error should be returned.",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						switch o := obj.(type) {
						case *apisv1alpha1.ProviderConfig:
							o.Spec.Credentials.Source = "Secret"
							o.Spec.Credentials.SecretRef = &xpv1.SecretKeySelector{
								Key: "creds",
							}
						case *corev1.Secret:
							return errors.New("no secret")
						}
						return nil
					},
				},
				usage: resource.NewProviderConfigUsageTracker(&fake.MockApplicator{}, &apisv1alpha1.ProviderConfigUsage{}),
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, nil
				},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					Spec: v1alpha1.VhostSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &common.ProviderConfigReference{
								Name: "testing-provider-config",
								Kind: "ProviderConfig",
							},
						},
					},
				},
			},
			want: errors.Wrap(errors.New("no secret"), "cannot get credentials: cannot get credentials secret"),
		},
		"Error in RabbitmqService": {
			reason: "Error should be returned.",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						switch o := obj.(type) {
						case *apisv1alpha1.ProviderConfig:
							o.Spec.Credentials.Source = "Secret"
							o.Spec.Credentials.SecretRef = &xpv1.SecretKeySelector{
								Key: "creds",
							}
						case *corev1.Secret:
							o.Data = map[string][]byte{
								"creds": []byte("{\"APIKey\":\"foo\",\"Email\":\"foo@bar.com\"}"),
							}
						}
						return nil
					},
				},
				usage: resource.NewProviderConfigUsageTracker(&fake.MockApplicator{}, &apisv1alpha1.ProviderConfigUsage{}),
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, errors.New("error in RabbitmqService NewClient")
				},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					Spec: v1alpha1.VhostSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &common.ProviderConfigReference{
								Name: "testing-provider-config",
								Kind: "ProviderConfig",
							},
						},
					},
				},
			},
			want: errors.Wrap(errors.New("error in RabbitmqService NewClient"), "cannot create new Service"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &connector{
				kube:         tc.fields.kube,
				usage:        tc.fields.usage,
				newServiceFn: tc.fields.newServiceFn,
			}
			_, err := e.Connect(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Connect(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestObserve(t *testing.T) {
	vHostTestName := "example-vhost"
	defaultQueueType := "classic"
	defaultTracing := false
	defaultDescription := "example-description"
	modifiedDescription := "modified-description"
	defaultClientError := rabbithole.ErrorResponse{StatusCode: http.StatusBadGateway, Message: "error"}

	type fields struct {
		service *rabbitmqclient.RabbitMqService
		logger  logging.Logger
	}

	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		o   managed.ExternalObservation
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Vhost exists and is up to date": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetVhost: func(name string) (rec *rabbithole.VhostInfo, err error) {
							rec = &rabbithole.VhostInfo{
								Name:             vHostTestName,
								DefaultQueueType: defaultQueueType,
								Tracing:          defaultTracing,
								Description:      defaultDescription,
							}
							return rec, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &defaultDescription,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceLateInitialized: false,
					ResourceUpToDate:        true,
				},
			},
		},
		"Vhost exists but is not up to date (description)": {
			reason: "Should detect that Description is not up to date and return ResourceUpToDate false",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetVhost: func(name string) (rec *rabbithole.VhostInfo, err error) {
							rec = &rabbithole.VhostInfo{
								Name:             vHostTestName,
								DefaultQueueType: defaultQueueType,
								Tracing:          defaultTracing,
								Description:      defaultDescription,
							}
							return rec, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &modifiedDescription,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceLateInitialized: false,
					ResourceUpToDate:        false,
				},
			},
		},
		"Vhost does not exist": {
			reason: "Should return ResourceExists false if vhost is not found",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetVhost: func(name string) (rec *rabbithole.VhostInfo, err error) {
							return nil, rabbithole.ErrorResponse{
								StatusCode: http.StatusNotFound,
								Message:    "vhost not found",
							}
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &defaultDescription,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
			},
		},
		"Client error": {
			reason: "Should return error if client returns error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetVhost: func(name string) (rec *rabbithole.VhostInfo, err error) {
							return nil, defaultClientError
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &defaultDescription,
							},
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(defaultClientError, errGetFailed),
			},
		},
		"Vhost is managed by another kubernetes resource": {
			reason: "Should return ResourceManagedFailed error when resource exist but is managed by another resource",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetVhost: func(name string) (rec *rabbithole.VhostInfo, err error) {
							rec = &rabbithole.VhostInfo{
								Name:             vHostTestName,
								DefaultQueueType: defaultQueueType,
								Tracing:          defaultTracing,
								Description:      defaultDescription,
							}
							return rec, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &defaultDescription,
							},
						},
					},
				},
			},
			want: want{
				err: rabbitmqmeta.NewNotCrossplaneManagedError(vHostTestName),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, log: tc.fields.logger}
			got, err := e.Observe(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	vHostTestName := "example-vhost"
	defaultQueueType := "classic"
	defaultTracing := false
	defaultDescription := "example-description"

	type fields struct {
		service *rabbitmqclient.RabbitMqService
		logger  logging.Logger
	}

	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		o   managed.ExternalCreation
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Create succeeds": {
			reason: "Should return managed.ExternalCreation{} on success",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutVhost: func(name string, settings rabbithole.VhostSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 201, Body: fake.MockReadCloser{MockClose: func() error { return nil }}}
							return res, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &defaultDescription,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"Create with provided vhost settings": {
			reason: "Should pass DefaultQueueType and Description to the client",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutVhost: func(name string, settings rabbithole.VhostSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 200,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return nil
									},
								}}
							return res, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &defaultDescription,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"Create fails": {
			reason: "Should return error if client returns error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutVhost: func(name string, settings rabbithole.VhostSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 500}
							return res, errors.New("create error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &defaultDescription,
							},
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.New("create error"), errCreateFailed),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, log: tc.fields.logger}
			got, err := e.Create(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	vHostTestName := "example-vhost"
	defaultQueueType := "classic"
	defaultTracing := false
	defaultDescription := "example-description"
	modifiedDescription := "modified-description"

	type fields struct {
		service *rabbitmqclient.RabbitMqService
		logger  logging.Logger
	}

	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		o   managed.ExternalUpdate
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Update succeeds": {
			reason: "Should return managed.ExternalUpdate{} on success",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutVhost: func(name string, settings rabbithole.VhostSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 204,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return nil
									},
								}}
							return res, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &modifiedDescription,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
		"Update fails": {
			reason: "Should return error if client returns error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutVhost: func(name string, settings rabbithole.VhostSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 500}
							return res, errors.New("update error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &defaultDescription,
							},
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.New("update error"), errUpdateFailed),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, log: tc.fields.logger}
			got, err := e.Update(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	vHostTestName := "example-vhost"
	defaultQueueType := "classic"
	defaultTracing := false
	defaultDescription := "example-description"

	type fields struct {
		service *rabbitmqclient.RabbitMqService
		logger  logging.Logger
	}

	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		o   managed.ExternalDelete
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Delete succeeds": {
			reason: "We should return managed.ExternalDelete{} on success",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteVhost: func(name string) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 204,
								Body: fake.MockReadCloser{
									MockClose: func() error {
										return nil
									},
								}}
							return res, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &defaultDescription,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalDelete{},
			},
		},
		"Delete fails": {
			reason: "Should return error if delete operation fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteVhost: func(name string) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 403,
								Body: fake.MockReadCloser{
									MockClose: func() error {
										return nil
									},
								}}
							return res, errors.New("delete error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Vhost{
					Spec: v1alpha1.VhostSpec{
						ForProvider: v1alpha1.VhostParameters{
							HostName: &vHostTestName,
							VhostSettings: &v1alpha1.VhostSettings{
								DefaultQueueType: &defaultQueueType,
								Tracing:          &defaultTracing,
								Description:      &defaultDescription,
							},
						},
					},
				},
			},
			want: want{
				o:   managed.ExternalDelete{},
				err: errors.Wrap(errors.New("delete error"), errDeleteFailed),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, log: tc.fields.logger}
			got, err := e.Delete(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}
