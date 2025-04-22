# Development Guide for provider-rabbitmq

This document provides instructions for developers who want to contribute to the provider-rabbitmq Crossplane provider.

## Getting Started

1. Use this repository as a template to create a new one.
2. Run `make submodules` to initialize the "build" Make submodule we use for CI/CD.
3. Rename the provider by running the following command:
```shell
  export provider_name=MyProvider # Camel case, e.g. GitHub
  make provider.prepare provider=${provider_name}
```
4. Add your new type by running the following command:
```shell
  export group=sample # lower case e.g. core, cache, database, storage, etc.
  export type=MyType # Camel case e.g. Bucket, Database, CacheCluster, etc.
  make provider.addtype provider=${provider_name} group=${group} kind=${type}
```
5. Replace the *sample* group with your new group in apis/{provider}.go
6. Replace the *mytype* type with your new type in internal/controller/{provider}.go
7. Replace the default controller and ProviderConfig implementations with your own
8. Run `make reviewable` to run code generation, linters, and tests.
9. Run `make build` to build the provider.

## Example Commands

These commands were used to create this provider:

```shell
make provider.prepare provider=RabbitMq
make provider.addtype provider=RabbitMq group=core kind=Vhost
make generate
```

## References

Refer to Crossplane's [CONTRIBUTING.md](https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md) file for more information on how the Crossplane community prefers to work. The [Provider Development](https://github.com/crossplane/crossplane/blob/master/contributing/guide-provider-development.md) guide may also be of use.