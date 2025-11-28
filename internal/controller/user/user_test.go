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
	"net/http"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	apisv1alpha1 "github.com/pnowy/provider-rabbitmq/apis/v1alpha1"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pnowy/provider-rabbitmq/apis/core/v1alpha1"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqclient/fake"
	"github.com/pnowy/provider-rabbitmq/internal/rabbitmqmeta"
)

func TestObserve(t *testing.T) {
	username := "test-user"
	password := "test-password"
	tags := []string{"administrator"}
	defaultClientError := rabbithole.ErrorResponse{StatusCode: http.StatusBadGateway, Message: "error"}

	type fields struct {
		kube    client.Client
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
		"NotUser": {
			reason: "Should return error if managed resource is not a User",
			fields: fields{
				logger: &fake.MockLog{},
			},
			args: args{
				mg: nil,
			},
			want: want{
				err: errors.New(errNotUser),
			},
		},
		"UserExists": {
			reason: "Should return ResourceExists true for passwordless user",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetUser: func(username string) (rec *rabbithole.UserInfo, err error) {
							rec = &rabbithole.UserInfo{
								Name: username,
							}
							return rec, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username:     &username,
							UserSettings: &v1alpha1.UserSettings{},
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
		"UserExistsDirectPassword": {
			reason: "Should return ResourceExists true for user with direct password",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetUser: func(username string) (rec *rabbithole.UserInfo, err error) {
							rec = &rabbithole.UserInfo{
								Name:             username,
								PasswordHash:     rabbithole.Base64EncodedSaltedPasswordHashSHA256(password),
								HashingAlgorithm: rabbithole.HashingAlgorithmSHA256,
							}
							return rec, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								Password: &password,
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
		"UserExistsSecretPassword": {
			reason: "Should return ResourceExists true for user with secret password",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
						secret := obj.(*corev1.Secret)
						secret.Data = map[string][]byte{
							"password": []byte(password),
						}
						return nil
					},
				},
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetUser: func(username string) (rec *rabbithole.UserInfo, err error) {
							rec = &rabbithole.UserInfo{
								Name:             username,
								PasswordHash:     rabbithole.Base64EncodedSaltedPasswordHashSHA256(password),
								HashingAlgorithm: rabbithole.HashingAlgorithmSHA256,
								Tags:             tags,
							}
							return rec, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								PasswordSecretRef: &xpv1.SecretKeySelector{
									Key: "password",
									SecretReference: xpv1.SecretReference{
										Name:      "test-secret",
										Namespace: "test-namespace",
									},
								},
								Tags: tags,
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
		"UserDoesNotExist": {
			reason: "Should return ResourceExists false when user does not exist",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetUser: func(username string) (rec *rabbithole.UserInfo, err error) {
							return nil, rabbithole.ErrorResponse{
								StatusCode: http.StatusNotFound,
								Message:    "user not found",
							}
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
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
		"GetUserError": {
			reason: "Should return error when API call fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetUser: func(username string) (rec *rabbithole.UserInfo, err error) {
							return nil, defaultClientError
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(defaultClientError, errGetFailed),
			},
		},
		"SecretNotFound": {
			reason: "Should return error when password secret is not found",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
						return errors.New("secret not found")
					},
				},
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockGetUser: func(username string) (rec *rabbithole.UserInfo, err error) {
							rec = &rabbithole.UserInfo{
								Name:             username,
								PasswordHash:     rabbithole.Base64EncodedSaltedPasswordHashSHA256(password),
								HashingAlgorithm: rabbithole.HashingAlgorithmSHA256,
								Tags:             tags,
							}
							return rec, nil
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								PasswordSecretRef: &xpv1.SecretKeySelector{
									Key: "password",
									SecretReference: xpv1.SecretReference{
										Name:      "non-existent-secret",
										Namespace: "test-namespace",
									},
								},
							},
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.Wrap(errors.New("secret not found"), "cannot get password secret"), "cannot resolve password"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, log: tc.fields.logger, kube: tc.fields.kube}
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
	username := "test-user"
	password := "test-password"
	tags := []string{"administrator"}

	type fields struct {
		kube    client.Client
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
		"NotUser": {
			reason: "Should return error if managed resource is not a User",
			fields: fields{
				logger: &fake.MockLog{},
			},
			args: args{
				mg: nil,
			},
			want: want{
				err: errors.New(errNotUser),
			},
		},
		"CreatePasswordlessSucceeds": {
			reason: "Should return no error when creating passwordless user succeeds",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutUserWithoutPassword: func(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"CreateDirectPasswordSucceeds": {
			reason: "Should return no error when creating user with direct password succeeds",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutUser: func(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								Password: &password,
								Tags:     tags,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"CreateSecretPasswordSucceeds": {
			reason: "Should return no error when creating user with secret password succeeds",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
						secret := obj.(*corev1.Secret)
						secret.Data = map[string][]byte{
							"password": []byte(password),
						}
						return nil
					},
				},
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutUser: func(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								PasswordSecretRef: &xpv1.SecretKeySelector{
									Key: "password",
									SecretReference: xpv1.SecretReference{
										Name:      "test-secret",
										Namespace: "test-namespace",
									},
								},
								Tags: tags,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"CreatePasswordUserFails": {
			reason: "Should return error when creating user with password fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutUser: func(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
							return nil, errors.New("create error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								Password: &password,
								Tags:     tags,
							},
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.New("create error"), errCreateFailed),
			},
		},
		"CreatePasswordlessUserFails": {
			reason: "Should return error when creating passwordless user fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutUserWithoutPassword: func(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
							return nil, errors.New("create error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.New("create error"), errCreateFailed),
			},
		},
		"SecretNotFoundOnCreate": {
			reason: "Should return error when password secret is not found during creation",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
						return errors.New("secret not found")
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								PasswordSecretRef: &xpv1.SecretKeySelector{
									Key: "password",
									SecretReference: xpv1.SecretReference{
										Name:      "non-existent-secret",
										Namespace: "test-namespace",
									},
								},
							},
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.Wrap(errors.New("secret not found"), "cannot get password secret"), "cannot determine password"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, log: tc.fields.logger, kube: tc.fields.kube}
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
	username := "test-user"
	password := "test-password"
	tags := []string{"administrator"}

	type fields struct {
		kube    client.Client
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
		"NotUser": {
			reason: "Should return error if managed resource is not a User",
			fields: fields{
				logger: &fake.MockLog{},
			},
			args: args{
				mg: nil,
			},
			want: want{
				err: errors.New(errNotUser),
			},
		},
		"UpdatePasswordlessSucceeds": {
			reason: "Should return no error when updating passwordless user succeeds",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutUserWithoutPassword: func(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
						},
					},
				},
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
		"UpdateDirectPasswordSucceeds": {
			reason: "Should return no error when updating user with direct password succeeds",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutUser: func(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								Password: &password,
								Tags:     tags,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
		"UpdateSecretPasswordSucceeds": {
			reason: "Should return no error when updating user with secret password succeeds",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
						secret := obj.(*corev1.Secret)
						secret.Data = map[string][]byte{
							"password": []byte(password),
						}
						return nil
					},
				},
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutUser: func(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
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
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								PasswordSecretRef: &xpv1.SecretKeySelector{
									Key: "password",
									SecretReference: xpv1.SecretReference{
										Name:      "test-secret",
										Namespace: "test-namespace",
									},
								},
								Tags: tags,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
		"UpdatePasswordUserFails": {
			reason: "Should return error when updating user with password fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutUser: func(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
							return nil, errors.New("update error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							rabbitmqmeta.AnnotationKeyCrossplaneManaged: "true",
						},
					},
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								Password: &password,
								Tags:     tags,
							},
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.New("update error"), errUpdateFailed),
			},
		},
		"UpdatePasswordlessUserFails": {
			reason: "Should return error when updating passwordless user fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockPutUserWithoutPassword: func(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
							return nil, errors.New("update error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.New("update error"), errUpdateFailed),
			},
		},
		"SecretNotFoundOnUpdate": {
			reason: "Should return error when password secret is not found during update",
			fields: fields{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
						return errors.New("secret not found")
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								PasswordSecretRef: &xpv1.SecretKeySelector{
									Key: "password",
									SecretReference: xpv1.SecretReference{
										Name:      "non-existent-secret",
										Namespace: "test-namespace",
									},
								},
							},
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.Wrap(errors.New("secret not found"), "cannot get password secret"), "cannot determine password"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, log: tc.fields.logger, kube: tc.fields.kube}
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
	username := "test-user"
	password := "test-password"
	tags := []string{"administrator"}

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
		"NotUser": {
			reason: "Should return error if managed resource is not a User",
			fields: fields{
				logger: &fake.MockLog{},
			},
			args: args{
				mg: nil,
			},
			want: want{
				o:   managed.ExternalDelete{},
				err: errors.New(errNotUser),
			},
		},
		"DeletePasswordlessUserSucceeds": {
			reason: "Should return no error when deleting passwordless user succeeds",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteUser: func(username string) (res *http.Response, err error) {
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
				mg: &v1alpha1.User{
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
						},
					},
				},
			},
			want: want{
				o:   managed.ExternalDelete{},
				err: nil,
			},
		},
		"DeletePasswordUserSucceeds": {
			reason: "Should return no error when deleting user with password succeeds",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteUser: func(username string) (res *http.Response, err error) {
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
				mg: &v1alpha1.User{
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
							UserSettings: &v1alpha1.UserSettings{
								Password: &password,
								Tags:     tags,
							},
						},
					},
				},
			},
			want: want{
				o:   managed.ExternalDelete{},
				err: nil,
			},
		},
		"DeleteUserFails": {
			reason: "Should return error when deleting user fails",
			fields: fields{
				service: &rabbitmqclient.RabbitMqService{
					Rmqc: &fake.MockClient{
						MockDeleteUser: func(username string) (res *http.Response, err error) {
							return nil, errors.New("delete error")
						},
					},
				},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.User{
					Spec: v1alpha1.UserSpec{
						ForProvider: v1alpha1.UserParameters{
							Username: &username,
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
		"ErrNotUser": {
			reason: "Should return error if managed resource is not a User",
			fields: fields{
				logger: &fake.MockLog{},
			},
			args: args{
				mg: nil,
			},
			want: want{
				err: errors.New(errNotUser),
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
				mg: &v1alpha1.User{
					Spec: v1alpha1.UserSpec{
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
				mg: &v1alpha1.User{
					Spec: v1alpha1.UserSpec{
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
