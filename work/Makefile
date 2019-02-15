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
JAIL?=		

#
# Poudriere portstree used for building
#
PORTSTREE?=	default


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

.if empty(JAIL)
.error "JAIL variable is not set!"
.endif


all: start checkout prepare build clean finish

start:
	@printf "===========================================================================\n"
	@printf "JOB#:\t%s\n" ${JOB_ID}
	@printf "COMMIT:\t%s\n" ${COMMIT_ID}
	@printf "REPO:\t%s\n" ${REPO_URL}
	@printf "PORT:\t%s\n" ${JOB_PORT}
	@printf "===========================================================================\n"
	@printf "Start at %s\n" "`date`"

checkout: clean
	@printf "= C H E C K O U T =========================================================\n"
	git clone ${REPO_URL} ${REPODIR}
	git -C "${REPODIR}" checkout ${COMMIT_ID}

prepare:
	@printf "= P R E P A R E ===========================================================\n"
	poudriere ports -u ${PORTSTREE}
	zfs snapshot ${ZPORTSFS}@clean

	# TODO - full overlay!
	rm -rf ${PORTSPATH}/${JOB_PORT}
	cp -pr ${REPODIR}/${JOB_PORT} ${PORTSPATH}/${JOB_PORT}

build:
	@printf "= B U I L D ===============================================================\n"
	poudriere testport -j ${JAIL} -p ${PORTSTREE} ${JOB_PORT}

clean:
	@printf "= C L E A N ===============================================================\n"
	rm -rf ${REPODIR}
	@zfs rollback ${ZPORTSFS}@clean || true
	@zfs destroy ${ZPORTSFS}@clean || true

finish:
	@echo Finish at `date`

.PHONY: all start checkout prepare build clean finish
