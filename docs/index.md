# authd

authd is a versatile authentication service for Ubuntu, designed to seamlessly integrate with cloud identity providers like OpenID Connect and Entra ID. It offers a secure interface for system authentication, enabling cloud-based identity management. It can be used to support logins through both GDM and SSH.

authd features a modular structure, facilitating straightforward integration with different cloud services. This design aids in maintaining strong security and effective user authentication. It's well-suited for handling access to cloud identities, offering a balance of security and ease of use.

authd uses brokers to interface with cloud identity providers through a [DBus API](https://github.com/ubuntu/authd/blob/HEAD/internal/examplebroker/com.ubuntu.auth.ExampleBroker.xml). Currently only [MS Entra ID](https://learn.microsoft.com/en-us/entra/fundamentals/whatis) is supported. For development purposes, authd also provides an example broker to help you develop your own.

The [MS Entra ID broker](https://github.com/ubuntu/oidc-broker) allows you to authenticate against MS Entra ID using MFA and the device authentication flow.

---------

## In this documentation

<!-- NOTE: changed grid layout as there is only three cards -->
````{grid} 1 1 1 1

```{grid-item-card} [How-to guides](howto/index)
:link: howto/index
:link-type: doc

**Step-by-step guides** covering key operations and common tasks
```

````

````{grid} 1 1 2 2
:reverse:

```{grid-item-card} [Reference](reference/index)
:link: reference/index
:link-type: doc

**Technical information** on troubleshooting authd
```

```{grid-item-card} [Explanation](explanation/index)
:link: explanation/index
:link-type: doc

**Discussion** of product architecture
```

````

---------

## Project and community

authd is a member of the Ubuntu family. It’s an open source project that warmly welcomes community projects, contributions, suggestions, fixes and constructive feedback.

* [Code of conduct](https://ubuntu.com/community/ethos/code-of-conduct)
* [Contribute](/howto/contributing)

Thinking about using authd for your next project? Get in touch!

```{toctree}
:hidden:
:maxdepth: 2

authd <self>
How-to guides </howto/index>
Reference </reference/index>
Explanation </explanation/index>
```
