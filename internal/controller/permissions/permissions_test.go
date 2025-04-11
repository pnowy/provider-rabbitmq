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
	"errors"
	"testing"

	"net/http"

	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	pkgErrors "github.com/pkg/errors"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
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
		"Create (success)": {
			reason: "We should return ResourceExists: Since UserPermissions Exist in RabbitMQ server",
			fields: fields{

				service: &rabbitmqclient.RabbitMqService{Rmqc: &fake.MockClient{
					MockGetPermissionsIn: func(vhost, username string) (rec rabbithole.PermissionInfo, err error) {
						rec = rabbithole.PermissionInfo{
							User:      username,
							Vhost:     vhost,
							Configure: ".*",
							Write:     ".*",
							Read:      ".*",
						}
						return rec, err
					},
				}},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Permissions{
					Spec: v1alpha1.PermissionsSpec{
						ForProvider: v1alpha1.PermissionsParameters{
							User:  "test",
							Vhost: "test",
							PermissionSettings: &v1alpha1.PermissionSettings{
								Write:     ".*",
								Read:      ".*",
								Configure: ".*",
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
		"Resource late initialized": {
			reason: "We should return ResourceLateInitialized: true",
			fields: fields{

				service: &rabbitmqclient.RabbitMqService{Rmqc: &fake.MockClient{
					MockGetPermissionsIn: func(vhost, username string) (rec rabbithole.PermissionInfo, err error) {
						rec = rabbithole.PermissionInfo{
							User:      username,
							Vhost:     vhost,
							Configure: ".*",
							Write:     ".*",
							Read:      ".*",
						}
						return rec, err
					},
				}},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Permissions{
					Spec: v1alpha1.PermissionsSpec{
						ForProvider: v1alpha1.PermissionsParameters{
							User:  "test",
							Vhost: "test",
						},
					},
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceLateInitialized: true,
					ConnectionDetails:       managed.ConnectionDetails{},
				},
			},
		},
		"Resource DoesNotExist": {
			reason: "We should return ResourceExists: False since UserPermissions Does Not Exist in RabbitMQ server",
			fields: fields{

				service: &rabbitmqclient.RabbitMqService{Rmqc: &fake.MockClient{
					MockGetPermissionsIn: func(vhost, username string) (rec rabbithole.PermissionInfo, err error) {
						rec = rabbithole.PermissionInfo{}
						var errResp rabbithole.ErrorResponse
						errResp.StatusCode = http.StatusNotFound
						errResp.Message = "permissions not found"
						err = errResp
						return rec, err
					},
				}},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Permissions{
					Spec: v1alpha1.PermissionsSpec{
						ForProvider: v1alpha1.PermissionsParameters{
							User:  "test",
							Vhost: "test",
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
		"Resource Exists and is not up to date": {
			reason: "We should return ResourceExists: Since UserPermissions Exist in RabbitMQ server",
			fields: fields{

				service: &rabbitmqclient.RabbitMqService{Rmqc: &fake.MockClient{
					MockGetPermissionsIn: func(vhost, username string) (rec rabbithole.PermissionInfo, err error) {
						rec = rabbithole.PermissionInfo{
							User:      username,
							Vhost:     vhost,
							Configure: ".*",
							Write:     ".*",
							Read:      ".*",
						}
						return rec, err
					},
				}},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Permissions{
					Spec: v1alpha1.PermissionsSpec{
						ForProvider: v1alpha1.PermissionsParameters{
							User:  "test",
							Vhost: "test",
							PermissionSettings: &v1alpha1.PermissionSettings{
								Write:     ".*",
								Read:      ".*",
								Configure: "new-config",
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
					ConnectionDetails:       managed.ConnectionDetails{},
				},
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
		"Create (success)": {
			reason: "We should return ExternalCreation",
			fields: fields{

				service: &rabbitmqclient.RabbitMqService{Rmqc: &fake.MockClient{
					MockUpdatePermissionsIn: func(vhost, username string, permissions rabbithole.Permissions) (res *http.Response, err error) {
						res = &http.Response{StatusCode: 200,
							Body: fake.MockReadCloser{}}
						return res, err
					},
				}},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Permissions{
					Spec: v1alpha1.PermissionsSpec{
						ForProvider: v1alpha1.PermissionsParameters{
							User:  "test",
							Vhost: "test",
							PermissionSettings: &v1alpha1.PermissionSettings{
								Write:     ".*",
								Read:      ".*",
								Configure: ".*",
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
		"Create (error)": {
			reason: "We should return Error",
			fields: fields{

				service: &rabbitmqclient.RabbitMqService{Rmqc: &fake.MockClient{
					MockUpdatePermissionsIn: func(vhost, username string, permissions rabbithole.Permissions) (res *http.Response, err error) {
						res = &http.Response{StatusCode: 200,
							Body: fake.MockReadCloser{}}
						err = errors.New("error")
						return res, err
					},
				}},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Permissions{
					Spec: v1alpha1.PermissionsSpec{
						ForProvider: v1alpha1.PermissionsParameters{
							User:  "test",
							Vhost: "test",
							PermissionSettings: &v1alpha1.PermissionSettings{
								Write:     ".*",
								Read:      ".*",
								Configure: ".*",
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalCreation{
					ConnectionDetails: nil,
				},
				err: pkgErrors.Wrap(errors.New("error"), "cannot create new User Permissions"),
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
		"Update (success)": {
			reason: "We should return ExternalUpdate",
			fields: fields{

				service: &rabbitmqclient.RabbitMqService{Rmqc: &fake.MockClient{
					MockUpdatePermissionsIn: func(vhost, username string, permissions rabbithole.Permissions) (res *http.Response, err error) {
						res = &http.Response{StatusCode: 200,
							Body: fake.MockReadCloser{}}
						return res, err
					},
				}},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Permissions{
					Spec: v1alpha1.PermissionsSpec{
						ForProvider: v1alpha1.PermissionsParameters{
							User:  "test",
							Vhost: "test",
							PermissionSettings: &v1alpha1.PermissionSettings{
								Write:     ".*",
								Read:      ".*",
								Configure: ".*",
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
		"Update (error)": {
			reason: "We should return Error",
			fields: fields{

				service: &rabbitmqclient.RabbitMqService{Rmqc: &fake.MockClient{
					MockUpdatePermissionsIn: func(vhost, username string, permissions rabbithole.Permissions) (res *http.Response, err error) {
						res = &http.Response{StatusCode: 200,
							Body: fake.MockReadCloser{}}
						err = errors.New("error")
						return res, err
					},
				}},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Permissions{
					Spec: v1alpha1.PermissionsSpec{
						ForProvider: v1alpha1.PermissionsParameters{
							User:  "test",
							Vhost: "test",
							PermissionSettings: &v1alpha1.PermissionSettings{
								Write:     ".*",
								Read:      ".*",
								Configure: ".*",
							},
						},
					},
				},
			},
			want: want{
				o:   managed.ExternalUpdate{},
				err: pkgErrors.Wrap(errors.New("error"), "cannot update User Permissions"),
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
		"Delete (success)": {
			reason: "We should return ExternalDelete",
			fields: fields{

				service: &rabbitmqclient.RabbitMqService{Rmqc: &fake.MockClient{
					MockClearPermissionsIn: func(vhost, username string) (res *http.Response, err error) {
						res = &http.Response{StatusCode: 200,
							Body: fake.MockReadCloser{}}
						return res, err
					},
				}},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Permissions{
					Spec: v1alpha1.PermissionsSpec{
						ForProvider: v1alpha1.PermissionsParameters{
							User:  "test",
							Vhost: "test",
							PermissionSettings: &v1alpha1.PermissionSettings{
								Write:     ".*",
								Read:      ".*",
								Configure: ".*",
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalDelete{},
			},
		},
		"Delete (error)": {
			reason: "We should return ExternalDelete",
			fields: fields{

				service: &rabbitmqclient.RabbitMqService{Rmqc: &fake.MockClient{
					MockClearPermissionsIn: func(vhost, username string) (res *http.Response, err error) {
						res = &http.Response{StatusCode: 200,
							Body: fake.MockReadCloser{}}
						err = errors.New("error")
						return res, err
					},
				}},
				logger: &fake.MockLog{},
			},
			args: args{
				mg: &v1alpha1.Permissions{
					Spec: v1alpha1.PermissionsSpec{
						ForProvider: v1alpha1.PermissionsParameters{
							User:  "test",
							Vhost: "test",
							PermissionSettings: &v1alpha1.PermissionSettings{
								Write:     ".*",
								Read:      ".*",
								Configure: ".*",
							},
						},
					},
				},
			},
			want: want{
				o:   managed.ExternalDelete{},
				err: pkgErrors.Wrap(errors.New("error"), "cannot delete User Permissions"),
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
