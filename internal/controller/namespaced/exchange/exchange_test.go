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
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/pnowy/provider-rabbitmq/apis/namespaced/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/namespaced/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqmeta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"

	"net/http"

	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"

	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	pkgErrors "github.com/pkg/errors"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient/fake"
	corev1 "k8s.io/api/core/v1"
)

// Unlike many Kubernetes projects Crossplane does not use third party testing
// libraries, per the common Go test review comments. Crossplane encourages the
// use of table driven unit tests. The tests of the crossplane-runtime project
// are representative of the testing style Crossplane encourages.
//
// https://github.com/golang/go/wiki/TestComments
// https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md#contributing-code

func TestConnect(t *testing.T) {

	const (
		credentialsSource = "Secret"
	)
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
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
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
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &common.ProviderConfigReference{
								Name: "testing-provider-config",
								Kind: "ProviderConfig",
							},
						},
					},
				},
			},
			want: pkgErrors.Wrap(errors.New("no provider config"), "cannot get ProviderConfig"),
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
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &common.ProviderConfigReference{
								Name: "testing-provider-config",
								Kind: "ProviderConfig",
							},
						},
					},
				},
			},
			want: pkgErrors.Wrap(errors.New("no secret"), "cannot get credentials: cannot get credentials secret"),
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
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &common.ProviderConfigReference{
								Name: "testing-provider-config",
								Kind: "ProviderConfig",
							},
						},
					},
				},
			},
			want: pkgErrors.Wrap(errors.New("error in RabbitmqService NewClient"), "cannot create new Service"),
		},
		"Resource No found error ": {
			reason: "We should return error",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						switch o := obj.(type) {
						case *apisv1alpha1.ProviderConfig:
							o.Spec.Credentials.Source = credentialsSource
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
			args: args{},
			want: pkgErrors.New(errNotExchange),
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
	exchangeTestName := "example-exchange"
	exchangeVhost := "example-exchange-vhost"
	exchangeType := "fanout"
	exchangeDurable := true
	exchangeAutoDelete := true
	exchangeArguments := map[string]interface{}{
		"x-queue-type":  "classic",
		"x-message-ttl": 30000,
	}

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
		"Exchange exists and is up to date": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetExchange: func(vhost, name string) (rec *rabbithole.DetailedExchangeInfo, err error) {
							rec = &rabbithole.DetailedExchangeInfo{
								Name:       name,
								Vhost:      vhost,
								Type:       exchangeType,
								Durable:    exchangeDurable,
								AutoDelete: exchangeAutoDelete,
								Arguments:  exchangeArguments,
							}
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
								Arguments:  rabbitmqclient.ConvertInterfaceMapToStringMap(exchangeArguments),
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
					ConnectionDetails:       managed.ConnectionDetails{},
				},
			},
		},
		"Exchange exists and is not up to date (type)": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetExchange: func(vhost, name string) (rec *rabbithole.DetailedExchangeInfo, err error) {
							rec = &rabbithole.DetailedExchangeInfo{
								Name:       name,
								Vhost:      vhost,
								Type:       "direct",
								Durable:    exchangeDurable,
								AutoDelete: exchangeAutoDelete,
							}
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
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
					ConnectionDetails:       managed.ConnectionDetails{},
				},
			},
		},
		"Exchange exists and is not up to date (durable)": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetExchange: func(vhost, name string) (rec *rabbithole.DetailedExchangeInfo, err error) {
							rec = &rabbithole.DetailedExchangeInfo{
								Name:       name,
								Vhost:      vhost,
								Type:       exchangeType,
								Durable:    false,
								AutoDelete: exchangeAutoDelete,
							}
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
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
					ConnectionDetails:       managed.ConnectionDetails{},
				},
			},
		},
		"Exchange exists and is not up to date (autoDelete)": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetExchange: func(vhost, name string) (rec *rabbithole.DetailedExchangeInfo, err error) {
							rec = &rabbithole.DetailedExchangeInfo{
								Name:       name,
								Vhost:      vhost,
								Type:       exchangeType,
								Durable:    exchangeDurable,
								AutoDelete: false,
							}
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
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
					ConnectionDetails:       managed.ConnectionDetails{},
				},
			},
		},
		"Exchange doesn't exist": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetExchange: func(vhost, name string) (rec *rabbithole.DetailedExchangeInfo, err error) {
							var errResp rabbithole.ErrorResponse
							errResp.StatusCode = http.StatusNotFound
							errResp.Message = "exchange not found"
							err = errResp
							return nil, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
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
		"Resource No found error ": {
			reason: "We should return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{},
				},
				logger: &fake.MockLog{},
			},
			args: args{},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				err: pkgErrors.New(errNotExchange),
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

	exchangeTestName := "example-exchange"
	exchangeVhost := "example-exchange-vhost"
	exchangeType := "fanout"
	exchangeDurable := true
	exchangeAutoDelete := true

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
			reason: "We should return managed.ConnectionDetails{}",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareExchange: func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{
					ConnectionDetails: managed.ConnectionDetails{},
				},
			},
		},
		"Create succeeds (without settings)": {
			reason: "We should return managed.ConnectionDetails{}",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareExchange: func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:             &exchangeTestName,
							Vhost:            exchangeVhost,
							ExchangeSettings: nil,
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{
					ConnectionDetails: managed.ConnectionDetails{},
				},
			},
		},
		"Create succeeds (without type)": {
			reason: "We should return managed.ConnectionDetails{}",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareExchange: func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       nil,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{
					ConnectionDetails: managed.ConnectionDetails{},
				},
			},
		},
		"Create fails": {
			reason: "We should return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareExchange: func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 200,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return nil
									},
								}}
							err = errors.New("error")
							return res, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{
					ConnectionDetails: nil,
				},
				err: pkgErrors.Wrap(errors.New("error"), "cannot create new Exchange"),
			},
		},
		"Create succeeds with error body close": {
			reason: "We should return managed.ConnectionDetails{}",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareExchange: func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 200,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return errors.New("error")
									},
								}}
							return res, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{
					ConnectionDetails: managed.ConnectionDetails{},
				},
			},
		},
		"Resource No found error ": {
			reason: "We should return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{},
				},
				logger: &fake.MockLog{},
			},
			args: args{},
			want: want{
				o:   managed.ExternalCreation{},
				err: pkgErrors.New(errNotExchange),
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

	exchangeTestName := "example-exchange"
	exchangeVhost := "example-exchange-vhost"
	exchangeType := "fanout"
	exchangeDurable := true
	exchangeAutoDelete := true

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
			reason: "We should return managed.ConnectionDetails{}",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareExchange: func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalUpdate{
					ConnectionDetails: managed.ConnectionDetails{},
				},
			},
		},
		"Update fails": {
			reason: "We should return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareExchange: func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 200,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return nil
									},
								}}
							err = errors.New("error")
							return res, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalUpdate{
					ConnectionDetails: nil,
				},
				err: pkgErrors.Wrap(errors.New("error"), "cannot update Exchange"),
			},
		},
		"Update succeeds with error body close": {
			reason: "We should return managed.ConnectionDetails{}",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareExchange: func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 200,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return errors.New("error")
									},
								}}
							return res, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalUpdate{
					ConnectionDetails: managed.ConnectionDetails{},
				},
			},
		},
		"Resource No found error ": {
			reason: "We should return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{},
				},
				logger: &fake.MockLog{},
			},
			args: args{},
			want: want{
				o:   managed.ExternalUpdate{},
				err: pkgErrors.New(errNotExchange),
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

	exchangeTestName := "example-exchange"
	exchangeVhost := "example-exchange-vhost"
	exchangeType := "fanout"
	exchangeDurable := true
	exchangeAutoDelete := true

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
			reason: "We should return managed.ConnectionDetails{}",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteExchange: func(vhost, exchange string) (res *http.Response, err error) {
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
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
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
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteExchange: func(vhost, exchange string) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 403,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return nil
									},
								}}
							err = errors.New("error")
							return res, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o:   managed.ExternalDelete{},
				err: pkgErrors.Wrap(errors.New("error"), "cannot delete Exchange"),
			},
		},
		"Delete succeeds with error body close": {
			reason: "We should return managed.ConnectionDetails{}",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteExchange: func(vhost, exchange string) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 200,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return errors.New("error")
									},
								}}
							return res, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Exchange{
					Spec: v1alpha1.ExchangeSpec{
						ForProvider: v1alpha1.ExchangeParameters{
							Name:  &exchangeTestName,
							Vhost: exchangeVhost,
							ExchangeSettings: &v1alpha1.ExchangeSettings{
								Type:       &exchangeType,
								Durable:    &exchangeDurable,
								AutoDelete: &exchangeAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalDelete{},
			},
		},
		"Resource No found error ": {
			reason: "We should return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{},
				},
				logger: &fake.MockLog{},
			},
			args: args{},
			want: want{
				o:   managed.ExternalDelete{},
				err: pkgErrors.New(errNotExchange),
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
