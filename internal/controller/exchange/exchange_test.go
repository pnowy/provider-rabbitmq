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

	"net/http"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"

	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	pkgErrors "github.com/pkg/errors"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pnowy/provider-rabbitmq/apis/core/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient/fake"
)

// Unlike many Kubernetes projects Crossplane does not use third party testing
// libraries, per the common Go test review comments. Crossplane encourages the
// use of table driven unit tests. The tests of the crossplane-runtime project
// are representative of the testing style Crossplane encourages.
//
// https://github.com/golang/go/wiki/TestComments
// https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md#contributing-code

func TestObserve(t *testing.T) {

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
							}
							return rec, err
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
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service}
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
								Body: fake.MockReadCloser{}}
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
								Body: fake.MockReadCloser{}}
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
								Body: fake.MockReadCloser{}}
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
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareExchange: func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 200,
								Body: fake.MockReadCloser{}}
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
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service}
			got, err := e.Create(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
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
								Body: fake.MockReadCloser{}}
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
		"Create fails": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareExchange: func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
							res = &http.Response{StatusCode: 200,
								Body: fake.MockReadCloser{}}
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
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service}
			got, err := e.Update(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
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
								Body: fake.MockReadCloser{}}
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
								Body: fake.MockReadCloser{}}
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
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service}
			got, err := e.Delete(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}
