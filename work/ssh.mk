#
# Caronade worker Makefile using SSH
#

SSH_URL?=	nonexistent
SSH_DIR?=	/dev/null

all: remote

remote:
	ssh ${SSH_URL} -t '/bin/bash -l -c "make -C ${SSH_DIR}"'

.PHONY: remote
