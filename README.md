# caronade

A small and light tool to help with FreeBSD Ports CI (Continuous Integration).

Whenever you push some code to your Git repository caronade will
receive a webhook and create build jobs for the affected ports.
Those jobs will trigger poudriere testport build jobs and the
result will be reported back to your repository via the GitHub
Status API.


## Main features

* Simple to setup and maintain (really!)
* Webhook support ([GitHub](https://github.com/) and [Gitea](https://gitea.io/))
* Poudriere support for building
* Supports GitHub/Gitea integration (Status API)
* Webserver for logfiles with HTTPS support


## Requirements

* git repository (GitHub or Gitea) with your ports
* [poudriere](https://github.com/freebsd/poudriere) on ZFS


## Why caronade?

[FreeBSD Ports](https://www.freebsd.org/doc/en/books/porters-handbook/) are a great
and huge collection of 3rd party sofware. For people working with ports it is very
monotonic to do a lot of build testing to verify that your changes/new port builds
fine in many different combinations (FreeBSD versions, architectures, Port options etc.).

Caronade does the testing for you while you continue with your work.


## Is this redports?

Redports was an attempt to run a fully hosted FreeBSD Ports building
service for everyone. Sadly it was also very complex, hard to maintain
and time consuming to operate which is why it was discontinued after a
few years.
Caronade is an attempt to build a similar tool but as simple as possible
and for your own poudriere machine. So caronade is not a fully hosted
service.
