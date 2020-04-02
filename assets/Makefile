DESTDIR?=	${.CURDIR:tA}/../build

all: build

build: static/caronade.css static/caronade.min.css

clean:
	rm -f static/caronade.css static/caronade.min.css

static/caronade.css: caronade.sass sass/
	sassc caronade.sass $@

static/caronade.min.css: caronade.sass sass/
	sassc -t compressed caronade.sass $@

.PHONY: all build clean
