# caronade

[![Go Report Card](https://goreportcard.com/badge/github.com/decke/caronade)](https://goreportcard.com/report/github.com/decke/caronade) [![Chat on IRC](https://img.shields.io/badge/chat-freenode%20%23caronade-brightgreen.svg)](https://webchat.freenode.net/?channels=%23caronade)

A small and light tool to help with FreeBSD Ports CI (Continuous Integration).

Caronade will automatically create build jobs using poudriere and
portlint whenever you push changes to your Git based ports repository.


## Main features

* Simple to setup and maintain
* [GitHub](https://github.com/) integration (Webhooks, Status API)
* [Poudriere](https://github.com/freebsd/poudriere/wiki) support for building
* Portlint support to verify port files
* EMail notifications
* Built-in webserver for Web UI and logfiles with HTTPS support


## Getting Started

Caronade has an embedded HTTP(S) server which can receive Webhooks and
shows your build status and build logs. It will execute a Makefile which
runs the poudriere build so it expects that you have poudriere running
successfully on the same machine. Each poudriere jail needs to have his
own portstree to be able to run jobs in parallel.

### Requirements

* git repository (GitHub) with your ports
* [poudriere](https://github.com/freebsd/poudriere) on ZFS
* caronade needs to be reachable from the Internet

### Installation

There is a FreeBSD port available as `ports-mgmt/caronade`.

`pkg install caronade`

### Configuration

Edit `/usr/local/etc/caronade/caronade.yaml` as needed.

### GitHub Setup: Webhook

Create a new repository which only contains your ports (avoid forking the
full FreeBSD portstree) on GitHub.

A webhook needs to be created which does a HTTP POST request to your caronade
daemon.

Create the webhook from the repository webinterface
```
github: repository settings -> webhooks -> add webhook
  payload url: baseurl from caronade
  content type: application/json
  secret: same as below
  events: Just the push event
```

Test the webhook by pushing a commit to the repository.

### GitHub Setup: Status API (optional)

If you want caronade to integrate into the GitHub Webinterface for your
repository then you need to create an GitHub API Token for that.

```
github: user settings -> developer settings -> personal access tokens -> repo:status
```

The token needs to be set in `caronade.yaml`.

### Usage

Caronade parses the commit message and expects all commit messages to start
with `category/portname` which it will use to generate build jobs. If the
commit message contains a line `CI: yes|no` build jobs will be generated for
all or no queues. Per default if no such line is found build jobs are
generated for all queues specified in `default_queues` in `caronade.yaml`.


### Remote builders

It's also possible to run build jobs on a remote machine as long as a SSH
connection is available and caronade is also installed on both machines.
For security reasons please create a dedicated ssh key for this connection.

`~/.ssh/config:`
```
Host builder-azure
  User root
  HostName <VM-HOST-NAME>.cloudapp.azure.com
  IdentityFile ~/.ssh/id_rsa_<VM-SSH-KEY>
  IdentitiesOnly=yes
  SendEnv JOB_ID COMMIT_ID REPO_URL JOB_PORT JAIL_NAME PORTSTREE PORTSDIR
```

`/etc/ssh/sshd_config:` (on builder-azure)
```
...
# caronade
AcceptEnv JOB_ID COMMIT_ID REPO_URL JOB_PORT JAIL_NAME PORTSTREE PORTSDIR
```

`/usr/local/etc/caronade/caronade.yaml:`
```
queues:
- name: 12.1/amd64
  recipe: ssh
  environment:
    SSH_URL: builder-azure
    SSH_DIR: /usr/local/caronade/work/121amd64
    SSH_RECIPE: /usr/local/caronade/work/poudriere.mk
    JAIL_NAME: 121amd64
    PORTSTREE: 121amd64

- name: portlint
  recipe: ssh
  environment:
    SSH_URL: builder-azure
    SSH_DIR: /usr/local/caronade/work/portlint
    SSH_RECIPE: /usr/local/caronade/work/portlint.mk
    PORTSDIR: /usr/local/poudriere/ports/default
```


## FAQ

### Why caronade?

[FreeBSD Ports](https://www.freebsd.org/doc/en/books/porters-handbook/) are a great
and huge collection of 3rd party sofware. For people working with ports it is very
monotonous to do a lot of build testing to verify that your changes/new port builds
fine in many different combinations (FreeBSD versions, architectures, Port options etc.).

Caronade does the testing for you while you continue with your work.


### Is this redports?

Redports was an attempt to run a fully hosted FreeBSD Ports building
service for everyone. Sadly it was also very complex, hard to maintain
and time consuming to operate which is why it was discontinued after a
few years.
Caronade is an attempt to build a similar tool but as simple as possible
and for your own poudriere machine. So caronade is not a fully hosted
service.
