/*
Copyright 2020 The Crossplane Authors.

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

package controller

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	cBinding "github.com/pnowy/provider-rabbitmq/internal/controller/cluster/binding"
	cConfig "github.com/pnowy/provider-rabbitmq/internal/controller/cluster/config"
	cExchange "github.com/pnowy/provider-rabbitmq/internal/controller/cluster/exchange"
	cPermissions "github.com/pnowy/provider-rabbitmq/internal/controller/cluster/permissions"
	cQueue "github.com/pnowy/provider-rabbitmq/internal/controller/cluster/queue"
	cUser "github.com/pnowy/provider-rabbitmq/internal/controller/cluster/user"
	cVhost "github.com/pnowy/provider-rabbitmq/internal/controller/cluster/vhost"
	"github.com/pnowy/provider-rabbitmq/internal/controller/namespaced/binding"
	"github.com/pnowy/provider-rabbitmq/internal/controller/namespaced/config"
	"github.com/pnowy/provider-rabbitmq/internal/controller/namespaced/exchange"
	"github.com/pnowy/provider-rabbitmq/internal/controller/namespaced/permissions"
	"github.com/pnowy/provider-rabbitmq/internal/controller/namespaced/queue"
	"github.com/pnowy/provider-rabbitmq/internal/controller/namespaced/user"
	"github.com/pnowy/provider-rabbitmq/internal/controller/namespaced/vhost"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupGated Setup creates all RabbitMq controllers with the supplied logger and adds them to
// the supplied manager.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		config.Setup,
		binding.SetupGated,
		exchange.SetupGated,
		permissions.SetupGated,
		queue.SetupGated,
		user.SetupGated,
		vhost.SetupGated,
		cConfig.Setup,
		cBinding.SetupGated,
		cExchange.SetupGated,
		cPermissions.SetupGated,
		cQueue.SetupGated,
		cUser.SetupGated,
		cVhost.SetupGated,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
