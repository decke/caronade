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

all: start checkout build clean

start:
	@printf "===========================================================================\n"
	@printf "JOB#:\t%s\n" ${JOB_ID}
	@printf "COMMIT:\t%s\n" ${COMMIT_ID}
	@printf "REPO:\t%s\n" ${REPO_URL}
	@printf "PORT:\t%s\n" ${JOB_PORT}
	@printf "DATE:\t%s\n" "`date`"
	@printf "===========================================================================\n"

checkout: clean
	git clone ${REPO_URL} ${REPODIR}
	git -C "${REPODIR}" -c advice.detachedHead=false checkout ${COMMIT_ID}
	@echo

build:
	portlint -A ${REPODIR}/${JOB_PORT}

clean:
	rm -rf ${REPODIR}

.PHONY: all start checkout build clean
