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

package binding

import (
	"context"
	"errors"
	"testing"

	"net/http"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	pkgErrors "github.com/pkg/errors"
	"github.com/pnowy/provider-rabbitmq/apis/core/v1alpha1"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient/fake"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqmeta"
	corev1 "k8s.io/api/core/v1"
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

func TestConnect(t *testing.T) {

	const (
		credentialsSource = "Secret"
	)
	type fields struct {
		kube         client.Client
		usage        resource.Tracker
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
				usage: &fake.MockTracker{
					MockTrack: func(ctx context.Context, mg resource.Managed) error {
						return nil
					},
				},
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, nil
				},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &xpv1.Reference{
								Name: "testing-provider-config",
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
				usage: &fake.MockTracker{
					MockTrack: func(ctx context.Context, mg resource.Managed) error {
						return nil
					},
				},
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, nil
				},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &xpv1.Reference{
								Name: "testing-provider-config",
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
							o.Spec.Credentials.Source = credentialsSource
							o.Spec.Credentials.SecretRef = &xpv1.SecretKeySelector{
								Key: "creds",
							}
						case *corev1.Secret:
							return errors.New("no secret")
						}
						return nil
					},
				},
				usage: &fake.MockTracker{
					MockTrack: func(ctx context.Context, mg resource.Managed) error {
						return nil
					},
				},
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, nil
				},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &xpv1.Reference{
								Name: "testing-provider-config",
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
				usage: &fake.MockTracker{
					MockTrack: func(ctx context.Context, mg resource.Managed) error {
						return nil
					},
				},
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, errors.New("Error in RabbitmqService NewClient")
				},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &xpv1.Reference{
								Name: "testing-provider-config",
							},
						},
					},
				},
			},
			want: pkgErrors.Wrap(errors.New("Error in RabbitmqService NewClient"), "cannot create new Service"),
		},
		"Error in Track": {
			reason: "Error should be returned.",
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
				usage: &fake.MockTracker{
					MockTrack: func(ctx context.Context, mg resource.Managed) error {
						return errors.New("Error in Track")
					},
				},
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, errors.New("Error in RabbitmqService NewClient")
				},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &xpv1.Reference{
								Name: "testing-provider-config",
							},
						},
					},
				},
			},
			want: pkgErrors.Wrap(errors.New("Error in Track"), "cannot track ProviderConfig usage"),
		},
		"Binding No found error ": {
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
				usage: &fake.MockTracker{
					MockTrack: func(ctx context.Context, mg resource.Managed) error {
						return nil
					},
				},
				newServiceFn: func(creds []byte) (*rabbitmqclient.RabbitMqService, error) {
					return &rabbitmqclient.RabbitMqService{}, nil
				},
			},
			args: args{},
			want: pkgErrors.New(errNotBinding),
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
		"Binding exists and is up to date": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          exchange,
								Destination:     queue,
								DestinationType: "queue",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments: map[string]interface{}{
									"properties-key": "properties-value",
								},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},

					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
							"crossplane.io/external-name":               "my-vh/my-source/queue/my-destination/properties-key",
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
		"Binding exists and is up to date (exchange type)": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListExchangeBindingsBetween: func(vhost, source string, destination string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          source,
								Destination:     destination,
								DestinationType: "exchange",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments: map[string]interface{}{
									"properties-key": "properties-value",
								},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "exchange",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
							"crossplane.io/external-name":               "my-vh/my-source/queue/my-destination/properties-key",
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
		"Binding exists and is up to date (other)": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListBindingsIn: func(vhost string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          "my-source",
								Destination:     "my-destination",
								DestinationType: "other",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments: map[string]interface{}{
									"properties-key": "properties-value",
								},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "other",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
							"crossplane.io/external-name":               "my-vh/my-source/queue/my-destination/properties-key",
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
		"Binding exists and is up to date without Arguments": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          exchange,
								Destination:     queue,
								DestinationType: "queue",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments:       map[string]interface{}{},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments:       map[string]string{},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
							"crossplane.io/external-name":               "my-vh/my-source/queue/my-destination/properties-key",
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
		"Binding exists and is not up to date (Arguments)": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          exchange,
								Destination:     queue,
								DestinationType: "queue",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments: map[string]interface{}{
									"properties-key": "properties-value",
								},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments:       map[string]string{},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
							"crossplane.io/external-name":               "my-vh/my-source/queue/my-destination/properties-key",
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
		"Binding exists and is not up to date (RoutingKey)": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          exchange,
								Destination:     queue,
								DestinationType: "queue",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments:       map[string]interface{}{},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "wrong-routing-key",
							Arguments:       map[string]string{},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
							"crossplane.io/external-name":               "my-vh/my-source/queue/my-destination/properties-key",
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
		"Binding doesn't exist (Vhost, Source, Destination, DestinationType)": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "wrong-routing-key",
							Arguments:       map[string]string{},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
							"crossplane.io/external-name":               "my-vh/my-source/queue/my-destination/properties-key",
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
		"Binding doesn't exist (External Name ID) ": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          exchange,
								Destination:     queue,
								DestinationType: "queue",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments:       map[string]interface{}{},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "wrong-routing-key",
							Arguments:       map[string]string{},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
							"crossplane.io/external-name":               "unexpected",
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
		"Binding doesn't exist (PropertiesKey) ": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          exchange,
								Destination:     queue,
								DestinationType: "queue",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments:       map[string]interface{}{},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments:       map[string]string{},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
							"crossplane.io/external-name":               "my-vh/my-source/queue/my-destination/wrong-properties-key",
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
		"Binding list error ": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							return rec, errors.New("error listing bindings")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "wrong-routing-key",
							Arguments:       map[string]string{},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"rabbitmq.crossplane.io/managed": "true",
							"crossplane.io/external-name":    "my-vh/my-source/queue/my-destination/properties-key",
						},
					},
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				err: pkgErrors.Wrap(errors.New("error listing bindings"), "cannot get RabbitMq Binding"),
			},
		},
		"Binding IsNotCrossplaneManaged error ": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          exchange,
								Destination:     queue,
								DestinationType: "queue",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments:       map[string]interface{}{},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "wrong-routing-key",
							Arguments:       map[string]string{},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"crossplane.io/external-name": "my-vh/my-source/queue/my-destination/properties-key",
						},
					},
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				err: pkgErrors.Wrap(errors.New("external object is managed by other resource or import is required (external name: )"), ""),
			},
		},
		"Binding No found error ": {
			reason: "We should return ResourceExists and ResourceUpToDate to true",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          exchange,
								Destination:     queue,
								DestinationType: "queue",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments:       map[string]interface{}{},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				err: pkgErrors.New(errNotBinding),
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
			reason: "We should return managed.ExternalCreation{}",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareBinding: func(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error) {
							res = &http.Response{
								StatusCode: 200,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return nil
									},
								},
								Header: http.Header{
									"Location": []string{"queue/properties-key"},
								}}
							return res, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"crossplane.io/external-name": "default-ext-name",
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"Binding No found error ": {
			reason: "We should return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockListQueueBindingsBetween: func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
							rec = append(rec, rabbithole.BindingInfo{
								Vhost:           vhost,
								Source:          exchange,
								Destination:     queue,
								DestinationType: "queue",
								PropertiesKey:   "properties-key",
								RoutingKey:      "my-routing-key",
								Arguments:       map[string]interface{}{},
							})
							return rec, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{},
			want: want{
				o:   managed.ExternalCreation{},
				err: pkgErrors.New(errNotBinding),
			},
		},
		"Create fails": {
			reason: "We should return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareBinding: func(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error) {
							return res, errors.New("error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"crossplane.io/external-name": "default-ext-name",
						},
					},
				},
			},
			want: want{
				o:   managed.ExternalCreation{},
				err: pkgErrors.Wrap(errors.New("error"), errCreateFailed),
			},
		},
		"Create PathUnescape error": {
			reason: "We should return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareBinding: func(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error) {
							res = &http.Response{
								StatusCode: 200,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return nil
									},
								},
								Header: http.Header{
									"Location": []string{"123%45%6"},
								}}
							return res, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"crossplane.io/external-name": "default-ext-name",
						},
					},
				},
			},
			want: want{
				o:   managed.ExternalCreation{},
				err: pkgErrors.Wrap(errors.New("invalid URL escape \"%6\""), errCreateFailed),
			},
		},
		"Create BodyClose error": {
			reason: "We should not return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeclareBinding: func(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error) {
							res = &http.Response{
								StatusCode: 200,
								Body: fake.MockReadCloser{
									MockClose: func() (err error) {
										return errors.New("error")
									},
								},
								Header: http.Header{
									"Location": []string{"queue/properties-key"},
								}}
							return res, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"crossplane.io/external-name": "default-ext-name",
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{},
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
		"Update error": {
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
							return res, err
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"crossplane.io/external-name": "default-ext-name",
						},
					},
				},
			},
			want: want{
				o:   managed.ExternalUpdate{},
				err: pkgErrors.New(errorUpdateNotSupported),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, log: tc.fields.logger}
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
			reason: "We should return managed.ExternalDelete{}",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteBinding: func(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error) {
							res = &http.Response{
								StatusCode: 200,
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
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"crossplane.io/external-name": "default-ext-name",
						},
					},
				},
			},
			want: want{
				o: managed.ExternalDelete{},
			},
		},
		"Delete fails": {
			reason: "We should return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteBinding: func(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error) {
							return res, errors.New("error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"crossplane.io/external-name": "default-ext-name",
						},
					},
				},
			},
			want: want{
				o:   managed.ExternalDelete{},
				err: pkgErrors.Wrap(errors.New("error"), errDeleteFailed),
			},
		},
		"Delete error Close": {
			reason: "We should not return error",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteBinding: func(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error) {
							res = &http.Response{
								StatusCode: 200,
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
				mg: &v1alpha1.Binding{
					Spec: v1alpha1.BindingSpec{
						ForProvider: v1alpha1.BindingParameters{
							Source:          "my-source",
							Destination:     "my-destination",
							DestinationType: "queue",
							Vhost:           "my-vh",
							RoutingKey:      "my-routing-key",
							Arguments: map[string]string{
								"properties-key": "properties-value",
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"crossplane.io/external-name": "default-ext-name",
						},
					},
				},
			},
			want: want{
				o: managed.ExternalDelete{},
			},
		},
		"Binding No found error ": {
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
				err: pkgErrors.New(errNotBinding),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, log: tc.fields.logger}
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
