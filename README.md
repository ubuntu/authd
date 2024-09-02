# Canonical authd

[actions-image]: https://github.com/ubuntu/authd/actions/workflows/qa.yaml/badge.svg
[actions-url]: https://github.com/ubuntu/authd/actions?query=workflow%3AQA

[license-image]: https://img.shields.io/badge/License-GPL3.0-blue.svg

[codecov-image]: https://codecov.io/gh/ubuntu/authd/graph/badge.svg
[codecov-url]: https://codecov.io/gh/ubuntu/authd

[reference-documentation-image]: https://pkg.go.dev/badge/github.com/ubuntu/authd.svg
[reference-documentation-url]: https://pkg.go.dev/github.com/ubuntu/authd

[goreport-image]: https://goreportcard.com/badge/github.com/ubuntu/authd
[goreport-url]: https://goreportcard.com/report/github.com/ubuntu/authd

[![Code quality][actions-image]][actions-url]
[![License][license-image]](COPYING)
[![Code coverage][codecov-image]][codecov-url]
[![Go Report Card][goreport-image]][goreport-url]
[![Reference documentation][reference-documentation-image]][reference-documentation-url]

Authd is an authentication daemon for cloud-based identity providers. It helps
ensure the secure management of identity and access for Ubuntu machines
anywhere in the world, on desktop and the server. Authd's modular design makes
it a versatile authentication service that can integrate with multiple identity
providers. MS Entra ID is currently supported and support for several other
identity providers is under active development.

## Documentation

Documentation for authd is currently available as a [wiki](https://github.com/ubuntu/authd/wiki/01---Get-started-with-authd) that includes how-to guides on:

- [Installation](https://github.com/ubuntu/authd/wiki/02---How%E2%80%90to-install)
- [Configuration](https://github.com/ubuntu/authd/wiki/03---How%E2%80%90to-configure)
- Login through [GDM](https://github.com/ubuntu/authd/wiki/04---How%E2%80%90to-log-in-with-GDM) and [SSH](https://github.com/ubuntu/authd/wiki/05--How%E2%80%90to-log-in-over-SSH)

A reference for [troubleshooting](https://github.com/ubuntu/authd/wiki/06--Troubleshooting-reference) is also provided along with an explanation of authd's [architecture](https://github.com/ubuntu/authd/wiki/07-Architecture-explanation).

## Brokers

Authd uses brokers to interface with cloud identity providers through a [DBus API](https://github.com/ubuntu/authd/blob/HEAD/examplebroker/com.ubuntu.auth.ExampleBroker.xml).

Currently [MS Entra ID](https://learn.microsoft.com/en-us/entra/fundamentals/whatis) is supported as an identity provider. 
The [MS Entra ID broker](https://github.com/ubuntu/oidc-broker) allows you to authenticate against MS Entra ID using MFA and the device authentication flow.

For development purposes, authd also provides an example broker to help you develop your own.

## Get involved

This is an [open source](COPYING) project and we warmly welcome community contributions, suggestions, and constructive feedback. If you're interested in contributing, please take a look at our [Contribution guidelines](CONTRIBUTING.md) first.

- To report an issue, please file a bug report against our repository, using the **report an issue** template.
- For suggestions and constructive feedback, report a feature request bug report, using the **request a feature** template.

## Get in touch

We're friendly! You can find our community forum at [https://discourse.ubuntu.com](https://discourse.ubuntu.com) where we discuss feature plans, development news, issues, updates and troubleshooting.
