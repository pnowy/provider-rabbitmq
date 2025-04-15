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

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	"github.com/pnowy/provider-rabbitmq/apis/core/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient/fake"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqmeta"

	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"

	"github.com/google/go-cmp/cmp"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
