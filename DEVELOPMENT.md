# Development Guide for provider-rabbitmq

This document provides instructions for developers who want to contribute to the provider-rabbitmq Crossplane provider.

## Setup

Run `make submodules` to initialize the `build` Make submodule we use for CI/CD.

```shell
make submodules
```

Execute code generation:

```shell
make generate
```

Run against a Kubernetes cluster:

```shell
make run
```

Run `make dev-init` to initialize the development environment with Kind and Helm Crossplane:

```shell
make dev-init
```

Run `make reviewable` to run code generation, linters, and tests:

```shell
make reviewable
```

Run `make build` to build the provider:

```shell
make build
```

Build, push, and install:

```shell
make all
```

Build binary:

```shell
make build
```

## New types

In order to add a new type, run the following commands:

```shell
make provider.addtype provider=RabbitMq group=core kind=MyNewType
make generate
```

After that implement the controller for the new type.

## Integration Tests

The provider includes integration tests that test the operator against a real RabbitMQ instance. These tests verify that the operator can
create, update, and delete RabbitMQ resources correctly.

To run the integration tests:

```shell
make test-integration
```

This will:

1. Set up a Kubernetes cluster using kind
2. Install Crossplane
3. Install RabbitMQ
4. Install the RabbitMQ provider
5. Run the integration tests
6. Clean up the test environment

The integration tests use the uptest framework to run tests against the provider.

Update `UPTEST_INPUT_MANIFESTS` if you want to include extra manifests for tests.

## Cluster scope resources

The cluster scope resources is a kind-of copy namespaced resources with a couple of changes:

- different groups and scope for `apis/core/v1alpha1`:
    - `groupName=rabbitmq.crossplane.io` instead of `groupName=rabbitmq.m.crossplane.io`
    - `Group   = "rabbitmq.crossplane.io"` instead of `Group   = "rabbitmq.m.crossplane.io"`
    - `kubebuilder:resource:scope=Cluster` instead of `kubebuilder:resource:scope=Namespaced`
    - `xpv1.ResourceSpec` for `TypeSpec` used instead of `xpv2.ManagedResourceSpec`
- separated cluster-scoped `ProviderConfig` and `ProvidedConfigUsage` resources but re-used `ClusterProviderConfig` from group namespace
  specific `rabbitmq.m.crossplane.io`
- shared `ProviderCredentials` struct from namespace scoped `ProviderConfig` with cluster scoped `ClusterProviderConfig` in order to be able
  to re-use controller logic when retrieving credentials
- adjusted controller code to the v1 types used in apis

Plan for the future: create separate generator to migrate namespaced resources to cluster scoped resources.

## References

Refer to Crossplane's [CONTRIBUTING.md](https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md) file for more information on
how the Crossplane
community prefers to work.
The [Provider Development](https://github.com/crossplane/crossplane/blob/master/contributing/guide-provider-development.md) guide may
also be of use.