#
# Caronade worker Makefile using SSH
#

# Example caronade.yaml:
#
# queues:
# - name: 12.1/amd64
#  recipe: ssh
#  environment:
#    SSH_URL: your-builder
#    SSH_DIR: /usr/local/caronade/work/121amd64
#    SSH_RECIPE: /usr/local/caronade/work/poudriere.mk
#    JAIL_NAME: 121amd64
#    PORTSTREE: 121amd64
#
# - name: portlint
#  recipe: ssh
#  environment:
#    SSH_URL: your-builder
#    SSH_DIR: /usr/local/caronade/work/portlint
#    SSH_RECIPE: /usr/local/caronade/work/portlint.mk
#    PORTSDIR: /usr/local/poudriere/ports/default
#
# Example .ssh/config:
#
# Host your-builder
#  User root
#  HostName the.real-hostnam.com
#  IdentityFile ~/.ssh/id_rsa
#  IdentitiesOnly=yes
#  SendEnv JOB_ID COMMIT_ID REPO_URL JOB_PORT JAIL_NAME PORTSTREE PORTSDIR
#
# Example /etc/ssh/sshd_config on your-builder:
#
# AcceptEnv JOB_ID COMMIT_ID REPO_URL JOB_PORT JAIL_NAME PORTSTREE PORTSDIR

SSH_URL?=	nonexistent
SSH_DIR?=	/dev/null

all: remote

remote:
	@ssh ${SSH_URL} -T '/bin/sh -c "make -C ${SSH_DIR} -f ${SSH_RECIPE}"'

.PHONY: remote
