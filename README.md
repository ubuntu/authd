# Welcome to Authentication daemon for external Broker

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

This is the code repository for authd, an authentication daemon for cloud-based identity provider.

Authd is a versatile authentication service for Ubuntu, designed to seamlessly integrate with cloud identity providers like OpenID Connect and Entra ID. It offers a secure interface for system authentication, supporting cloud-based identity management.

Authd features a modular structure, facilitating straightforward integration with different cloud services. This design aids in maintaining strong security and effective user authentication. It's well-suited for handling access to cloud identities, offering a balance of security and ease of use.

Authd uses brokers to interface with cloud identity providers through a [DBus API](https://github.com/ubuntu/authd/blob/HEAD/examplebroker/com.ubuntu.auth.ExampleBroker.xml). Currently only [MS Entra ID](https://learn.microsoft.com/en-us/entra/fundamentals/whatis) is supported. For development purposes, Authd also provides an example broker to help you develop your own.

The [MS Entra ID broker](https://github.com/ubuntu/oidc-broker) allows you to authenticate against MS Entra ID using MFA and the device authentication flow.
 
## Installation and usage

The [documentation](https://github.com/ubuntu/authd/wiki/01---Get-started-with-authd) provides detailed instructions for installing, configuring, and using Authd and the MS Entra ID provider.

## Get involved

This is an [open source](COPYING) project and we warmly welcome community contributions, suggestions, and constructive feedback. If you're interested in contributing, please take a look at our [Contribution guidelines](CONTRIBUTING.md) first.

- to report an issue, please file a bug report against our repository, using a bug template.
- for suggestions and constructive feedback, report a feature request bug report, using the proposed template.

## Get in touch

We're friendly! We have a community forum at [https://discourse.ubuntu.com](https://discourse.ubuntu.com) where we discuss feature plans, development news, issues, updates and troubleshooting.
