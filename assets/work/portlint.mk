#
# Caronade worker Makefile for Portlint
#

#
# Job ID is an informative variable only
#
JOB_ID?=	UNKNOWN

#
# Git sha1 hash used for building (default HEAD)
#
COMMIT_ID?=	HEAD

#
# URL to Git Repository
#
REPO_URL?=	

#
# Portname to build in format "category/portname"
#
JOB_PORT?=	

# DON'T TOUCH ANYTHING BELOW HERE

WORKDIR?=	${.CURDIR:tA}
REPODIR?=	${WORKDIR}/repo.git

.if empty(REPO_URL)
.error "REPO_URL variable is not set!"
.endif

.if empty(JOB_PORT)
.error "JOB_PORT variable is not set!"
.endif

all: pre-clean checkout build post-clean

checkout:
	git clone ${REPO_URL} ${REPODIR}
	git -C "${REPODIR}" -c advice.detachedHead=false checkout ${COMMIT_ID}
	@echo

build:
	portlint -A ${REPODIR}/${JOB_PORT}

pre-clean post-clean:
	rm -rf ${REPODIR}

.PHONY: all checkout build pre-clean post-clean
