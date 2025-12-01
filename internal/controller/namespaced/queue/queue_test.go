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
	"net/http"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/pnowy/provider-rabbitmq/apis/namespaced/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/namespaced/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient/fake"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqmeta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Unlike many Kubernetes projects Crossplane does not use third party testing
// libraries, per the common Go test review comments. Crossplane encourages the
// use of table driven unit tests. The tests of the crossplane-runtime project
// are representative of the testing style Crossplane encourages.
//
// https://github.com/golang/go/wiki/TestComments
// https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md#contributing-code

func TestObserve(t *testing.T) {
	queueName := "test-queue"
	queueVhost := "test-vhost"
	queueType := "classic"
	queueDurable := true
	queueAutoDelete := false
	queueArguments := map[string]interface{}{
		"x-queue-type":  "classic",
		"x-message-ttl": 30000,
	}
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
		"NotQueue": {
			reason: "Should return error if managed resource is not a Queue",
			fields: fields{
				logger: &fake.MockLog{},
			},
			args: args{
				mg: nil,
			},
			want: want{
				err: errors.New(errNotQueue),
			},
		},
		"QueueExists": {
			reason: "Should return ResourceExists true when queue exists",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetQueue: func(vhost, queue string) (rec *rabbithole.DetailedQueueInfo, err error) {
							rec = &rabbithole.DetailedQueueInfo{
								Name:       queueName,
								Vhost:      queueVhost,
								Type:       queueType,
								Durable:    queueDurable,
								AutoDelete: rabbithole.AutoDelete(queueAutoDelete),
								Arguments:  queueArguments,
							}
							return rec, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Queue{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
							QueueSettings: &v1alpha1.QueueSettings{
								Type:       &queueType,
								Durable:    &queueDurable,
								AutoDelete: &queueAutoDelete,
								Arguments:  rabbitmqclient.ConvertInterfaceMapToStringMap(queueArguments),
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: false,
				},
			},
		},
		"QueueDoesNotExist": {
			reason: "Should return ResourceExists false when queue does not exist",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetQueue: func(vhost, queue string) (rec *rabbithole.DetailedQueueInfo, err error) {
							return nil, rabbithole.ErrorResponse{
								StatusCode: http.StatusNotFound,
								Message:    "queue not found",
							}
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Queue{
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
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
		"GetQueueError": {
			reason: "Should return error when API call fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetQueue: func(vhost, queue string) (rec *rabbithole.DetailedQueueInfo, err error) {
							return nil, defaultClientError
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Queue{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(defaultClientError, errGetFailed),
			},
		},
		"QueueNeedUpdate": {
			reason: "Should detect that queue settings are not up to date",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetQueue: func(vhost, queue string) (rec *rabbithole.DetailedQueueInfo, err error) {
							rec = &rabbithole.DetailedQueueInfo{
								Name:       queueName,
								Vhost:      queueVhost,
								Type:       queueType,
								Durable:    !queueDurable, // Different from spec
								AutoDelete: rabbithole.AutoDelete(queueAutoDelete),
							}
							return rec, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Queue{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
							QueueSettings: &v1alpha1.QueueSettings{
								Type:       &queueType,
								Durable:    &queueDurable,
								AutoDelete: &queueAutoDelete,
								Arguments:  map[string]string{},
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: false,
				},
			},
		},
		"NotCrossplaneManaged": {
			reason: "Should return error when queue exists but is not managed by Crossplane",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetQueue: func(vhost, queue string) (rec *rabbithole.DetailedQueueInfo, err error) {
							rec = &rabbithole.DetailedQueueInfo{
								Name:       queueName,
								Vhost:      queueVhost,
								Type:       queueType,
								Durable:    queueDurable,
								AutoDelete: rabbithole.AutoDelete(queueAutoDelete),
							}
							return rec, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Queue{
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
							QueueSettings: &v1alpha1.QueueSettings{
								Type:       &queueType,
								Durable:    &queueDurable,
								AutoDelete: &queueAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				err: rabbitmqmeta.NewNotCrossplaneManagedError(queueName),
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
	queueName := "test-queue"
	queueVhost := "test-vhost"
	queueType := "classic"
	queueDurable := true
	queueAutoDelete := false

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
		"NotQueue": {
			reason: "Should return error if managed resource is not a Queue",
			fields: fields{
				logger: &fake.MockLog{},
			},
			args: args{
				mg: nil,
			},
			want: want{
				err: errors.New(errNotQueue),
			},
		},
		"CreateSucceeds": {
			reason: "Should return ExternalCreation and no error when create succeeds",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareQueue: func(vhost, queue string, info rabbithole.QueueSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 201,
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
				mg: &v1alpha1.Queue{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
							QueueSettings: &v1alpha1.QueueSettings{
								Type:       &queueType,
								Durable:    &queueDurable,
								AutoDelete: &queueAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"CreateFails": {
			reason: "Should return error when create fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareQueue: func(vhost, queue string, info rabbithole.QueueSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 500}
							return res, errors.New("create error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Queue{
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
							QueueSettings: &v1alpha1.QueueSettings{
								Type:       &queueType,
								Durable:    &queueDurable,
								AutoDelete: &queueAutoDelete,
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
	queueName := "test-queue"
	queueVhost := "test-vhost"
	queueType := "classic"
	queueDurable := true
	queueAutoDelete := false

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
		"NotQueue": {
			reason: "Should return error if managed resource is not a Queue",
			fields: fields{
				logger: &fake.MockLog{},
			},
			args: args{
				mg: nil,
			},
			want: want{
				err: errors.New(errNotQueue),
			},
		},
		"UpdateSucceeds": {
			reason: "Should return ExternalUpdate and no error when update succeeds",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareQueue: func(vhost, queue string, info rabbithole.QueueSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.Queue{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
							QueueSettings: &v1alpha1.QueueSettings{
								Type:       &queueType,
								Durable:    &queueDurable,
								AutoDelete: &queueAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
		"UpdateFails": {
			reason: "Should return error when update fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareQueue: func(vhost, queue string, info rabbithole.QueueSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 500}
							return res, errors.New("update error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Queue{
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
							QueueSettings: &v1alpha1.QueueSettings{
								Type:       &queueType,
								Durable:    &queueDurable,
								AutoDelete: &queueAutoDelete,
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
	queueName := "test-queue"
	queueVhost := "test-vhost"
	queueType := "classic"
	queueDurable := true
	queueAutoDelete := false

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
		"NotQueue": {
			reason: "Should return error if managed resource is not a Queue",
			fields: fields{
				logger: &fake.MockLog{},
			},
			args: args{
				mg: nil,
			},
			want: want{
				err: errors.New(errNotQueue),
			},
		},
		"DeleteSucceeds": {
			reason: "Should return ExternalDelete and no error when delete succeeds",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteQueue: func(vhost, queue string, opts ...rabbithole.QueueDeleteOptions) (res *http.Response, err error) {
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
				mg: &v1alpha1.Queue{
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
							QueueSettings: &v1alpha1.QueueSettings{
								Type:       &queueType,
								Durable:    &queueDurable,
								AutoDelete: &queueAutoDelete,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalDelete{},
			},
		},
		"DeleteFails": {
			reason: "Should return error when delete fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteQueue: func(vhost, queue string, opts ...rabbithole.QueueDeleteOptions) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 500}
							return res, errors.New("delete error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Queue{
					Spec: v1alpha1.QueueSpec{
						ForProvider: v1alpha1.QueueParameters{
							Name:  &queueName,
							Vhost: queueVhost,
							QueueSettings: &v1alpha1.QueueSettings{
								Type:       &queueType,
								Durable:    &queueDurable,
								AutoDelete: &queueAutoDelete,
							},
						},
					},
				},
			},
			want: want{
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

func TestConnect(t *testing.T) {
	errBoom := errors.New("boom")

	type fields struct {
		kube         client.Client
		usage        *resource.ProviderConfigUsageTracker
		newServiceFn func(creds []byte) (*rabbitmqclient.RabbitMqService, error)
		logger       logging.Logger
	}

	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"ErrNotQueue": {
			reason: "Should return error if managed resource is not a Queue",
			fields: fields{
				logger: &fake.MockLog{},
			},
			args: args{
				mg: nil,
			},
			want: want{
				err: errors.New(errNotQueue),
			},
		},
		"ErrGetProviderConfig": {
			reason: "Should return error if we can't get ProviderConfig",
			fields: fields{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(errBoom),
				},
				usage:  resource.NewProviderConfigUsageTracker(&fake.MockApplicator{}, &apisv1alpha1.ProviderConfigUsage{}),
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Queue{
					Spec: v1alpha1.QueueSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &common.ProviderConfigReference{
								Name: "testing-provider-config",
								Kind: "ProviderConfig",
							},
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errBoom, errGetPC),
			},
		},
		"SuccessfulConnect": {
			reason: "Should successfully connect and create external client",
			fields: fields{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						switch o := obj.(type) {
						case *apisv1alpha1.ProviderConfig:
							o.Spec.Credentials.Source = xpv1.CredentialsSourceSecret
							o.Spec.Credentials.CommonCredentialSelectors = xpv1.CommonCredentialSelectors{
								SecretRef: &xpv1.SecretKeySelector{
									Key: "credentials",
								},
							}
						case *corev1.Secret:
							o.Data = map[string][]byte{
								"creds": []byte("{\"APIKey\":\"foo\",\"Email\":\"foo@bar.com\"}"),
							}
						}
						return nil
					}),
				},
				usage: resource.NewProviderConfigUsageTracker(&fake.MockApplicator{}, &apisv1alpha1.ProviderConfigUsage{}),
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, nil
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Queue{
					Spec: v1alpha1.QueueSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &common.ProviderConfigReference{
								Name: "testing-provider-config",
								Kind: "ProviderConfig",
							},
						},
					},
				},
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := &connector{
				kube:         tc.fields.kube,
				usage:        tc.fields.usage,
				newServiceFn: tc.fields.newServiceFn,
				logger:       tc.fields.logger,
			}
			_, err := c.Connect(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nc.Connect(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
		})
	}
}
