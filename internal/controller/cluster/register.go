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

package cluster

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/pnowy/provider-rabbitmq/internal/controller/cluster/binding"
	"github.com/pnowy/provider-rabbitmq/internal/controller/cluster/config"
	"github.com/pnowy/provider-rabbitmq/internal/controller/cluster/exchange"
	"github.com/pnowy/provider-rabbitmq/internal/controller/cluster/permissions"
	"github.com/pnowy/provider-rabbitmq/internal/controller/cluster/queue"
	"github.com/pnowy/provider-rabbitmq/internal/controller/cluster/user"
	"github.com/pnowy/provider-rabbitmq/internal/controller/cluster/vhost"
	ctrl "sigs.k8s.io/controller-runtime"
)

func Setup(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		config.Setup,
		binding.Setup,
		exchange.Setup,
		permissions.Setup,
		queue.Setup,
		user.Setup,
		vhost.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

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
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
