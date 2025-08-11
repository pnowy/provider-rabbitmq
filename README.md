# Crossplane Provider for RabbitMQ

`provider-rabbitmq` is a [Crossplane](https://crossplane.io/) provider that enables management of RabbitMQ resources through Kubernetes custom resources. This
provider allows you to manage RabbitMQ resources using Kubernetes-style declarative configurations.

## Overview

The RabbitMQ provider offers the following features:

- **Virtual Hosts**: Manage virtual hosts
- **Users**: Manage users
- **Exchanges**: Manage exchanges
- **Queues**: Manage queues
- **Bindings**: Configure bindings between exchanges and queues
- **Permissions**: Manage user permissions within virtual hosts

Planned features:

- **Policies**: Manage RabbitMQ policies for queues and exchanges
- **Operator policies**: Manage operator policies for queues
- **Topic permissions**: Manage a user's set of topic permissions
- **Federation upstream**: Manage federation upstream
- **Shovel**: Manage dynamic shovel

## Getting Started

### Installation

To install the provider, use the following resource definition (replace `PROVIDER_VERSION` with the desired version):

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-rabbitmq
  namespace: crossplane-system
spec:
  package: xpkg.upbound.io/pnowy/provider-rabbitmq:PROVIDER_VERSION
```

This will install the provider in the `crossplane-system` namespace and install CRDs and controllers for the provider.

### Deployment runtime config (optional)

In order to configure the provider deployment runtime config, create a `DeploymentRuntimeConfig` resource:

```yaml
apiVersion: pkg.crossplane.io/v1beta1
kind: DeploymentRuntimeConfig
metadata:
  name: provider-rabbitmq
spec:
  deploymentTemplate:
    spec:
      template:
        spec:
          tolerations:
            - effect: NoSchedule
              key: crossplane
              operator: Exists
```

which can be referenced in the provider definition:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-rabbitmq
  namespace: crossplane-system
spec:
  package: xpkg.upbound.io/pnowy/provider-rabbitmq:PROVIDER_VERSION
  runtimeConfigRef:
    apiVersion: pkg.crossplane.io/v1beta1
    kind: DeploymentRuntimeConfig
    name: provider-rabbitmq
```

### Setup

The provider requires credentials to connect to your RabbitMQ server. Create a Kubernetes secret with your RabbitMQ credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: rabbitmq-credentials
  namespace: crossplane-system
stringData:
  credentials: |
    {
      "username": "guest",
      "password": "guest",
      "endpoint": "http://rabbitmq.rabbitmq.svc.cluster.local:15672"
    }
```

Then create a `ProviderConfig` to use these credentials:

```yaml
apiVersion: rabbitmq.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: provider-rabbitmq
spec:
  credentials:
    source: Secret
    secretRef:
      name: rabbitmq-credentials
      namespace: crossplane-system
      key: credentials
```

- For each RabbitMQ instance you need one or more `ProviderConfig` resources
- The `ProviderConfig` resource is used to store the RabbitMQ endpoint, username and password details that are required to connect to the RabbitMQ API server

## Custom Resource Definitions

You can explore the available custom resources:

- `kubectl get crd | grep rabbitmq.crossplane.io` to list all the CRDs provided by the provider
- `kubectl explain <CRD_NAME>` for docs on the CLI
- You can also see the CRDs in the `package/crds` directory

## Usage Examples

You will find usage examples in the [examples](examples) directory.

## Quickstart

If you would like to quickstart with the provider you can use `make quickstart` command.

The method is a `Makefile` target that sets up a complete development environment for testing and using a RabbitMQ provider for Crossplane.

1. **Creates a local Kubernetes cluster** using Kind (Kubernetes in Docker) named "crossplane-rabbitmq-quickstart"
2. **Sets up the Kubernetes context** to point to the newly created cluster
3. **Installs Crossplane using Helm in a dedicated "crossplane-system" namespace** (a Kubernetes add-on for managing infrastructure resources)
4. **Installs RabbitMQ using Helm in a dedicated "rabbitmq" namespace**
5. **Installs Provider RabbitMQ**:
    - Applies the provider specification from `examples/provider/provider.yaml`
    - Applies configuration from `examples/provider/config.yaml`

This method provides developers with a one-command solution to set up a complete development environment with all necessary components for working with the
Crossplane RabbitMQ provider.

As a next step you can apply some examples from the [examples/sample](examples/sample) directory (e.g. `vhost.yaml`). To log in into RabbitMQ admin
console from localhost execute the command: `k port-forward svc/rabbitmq 15672:15672 -n rabbitmq` and then open http://localhost:15672 in your browser. Login
with username `guest` and password `guest`.

## Development

For information on how to contribute to this provider, please see the [Development Guide](DEVELOPMENT.md).
