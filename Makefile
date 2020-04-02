PREFIX?=	/usr/local
DESTDIR?=	${.CURDIR:tA}/stage

ETCDIR?=	${PREFIX}/etc
BINDIR?=	${PREFIX}/bin
DATADIR?=	${PREFIX}/caronade

all: build

assets:
	@make -C assets

build: assets
	go build -v -o caronade ./cmd/caronade/...

clean:
	@make -C assets clean
	rm -f caronade

install: assets
	mkdir -p $(DESTDIR)$(BINDIR)
	cp -p caronade $(DESTDIR)$(BINDIR)/

	mkdir -p $(DESTDIR)$(ETCDIR)
	cp -p configs/caronade.yaml $(DESTDIR)$(ETCDIR)/caronade.yaml.sample

	mkdir -p $(DESTDIR)$(DATADIR)
	mkdir -p $(DESTDIR)$(DATADIR)/builds
	cp -pr assets/static $(DESTDIR)$(DATADIR)/
	cp -pr assets/templates $(DESTDIR)$(DATADIR)/
	cp -pr assets/work $(DESTDIR)$(DATADIR)/

.PHONY: all assets build clean install
