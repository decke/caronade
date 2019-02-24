#
# Caronade worker Makefile for Ports CI with poudriere
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

#
# Poudriere jail used for building
#
JAIL_NAME?=		

#
# Poudriere portstree used for building
#
PORTSTREE?=	


# DON'T TOUCH ANYTHING BELOW HERE

WORKDIR?=	${.CURDIR:tA}
REPODIR?=	${WORKDIR}/repo.git

ZPOOL?=		zroot
ZROOTFS?=	/poudriere
ZPORTSFS?=	${ZPOOL}${ZROOTFS}/ports/${PORTSTREE}
PORTSPATH!=	zfs get -H mountpoint ${ZPORTSFS} | cut -f3


.if empty(REPO_URL)
.error "REPO_URL variable is not set!"
.endif

.if empty(JOB_PORT)
.error "JOB_PORT variable is not set!"
.endif

.if empty(JAIL_NAME)
.error "JAIL_NAME variable is not set!"
.endif


all: checkout prepare build clean

checkout: clean
	git clone ${REPO_URL} ${REPODIR}
	git -C "${REPODIR}" -c advice.detachedHead=false checkout ${COMMIT_ID}

prepare:
	poudriere ports -u ${PORTSTREE}
	zfs snapshot ${ZPORTSFS}@clean

	# TODO - full overlay!
	rm -rf ${PORTSPATH}/${JOB_PORT}
	cp -pr ${REPODIR}/${JOB_PORT} ${PORTSPATH}/${JOB_PORT}

build:
	poudriere testport -j ${JAIL_NAME} -p ${PORTSTREE} ${JOB_PORT}

clean:
	rm -rf ${REPODIR}
	@zfs rollback ${ZPORTSFS}@clean || true
	@zfs destroy ${ZPORTSFS}@clean || true

.PHONY: all checkout prepare build clean
