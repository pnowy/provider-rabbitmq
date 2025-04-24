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

## References

Refer to Crossplane's [CONTRIBUTING.md](https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md) file for more information on how the Crossplane community prefers to work. The [Provider Development](https://github.com/crossplane/crossplane/blob/master/contributing/guide-provider-development.md) guide may also be of use.