---
myst:
  html_meta:
    "description lang=en":
      "authd is an authentication service for Ubuntu, offering integration with multiple cloud identity providers, including Google IAM and Microsoft Entra ID."
---

# authd

authd is an authentication service for Ubuntu that integrates with multiple
cloud identity providers. It offers a secure interface for system
authentication, enabling cloud-based identity management for Ubuntu Desktop and
Server.

authd has a modular design, comprising an authentication daemon and various
identity brokers. This enables authd to support a growing list of cloud
identity providers. Currently, authd supports authentication with both [MS
Entra ID](https://learn.microsoft.com/en-us/entra/fundamentals/whatis) and
[Google IAM](https://cloud.google.com/iam/docs/overview). An example broker is
also provided to help developers create new brokers for additional identity
services.

If an organization is pursuing cloud-based authentication of Ubuntu
workstations and servers, authd is a secure and versatile service to support a
full transition to the cloud.

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

Documentation for the [stable](https://canonical-authd.readthedocs-hosted.com/en/stable/) release of authd and the [latest](https://canonical-authd.readthedocs-hosted.com/en/latest/) development version are
both available.

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
