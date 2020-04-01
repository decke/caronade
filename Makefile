DESTDIR?=build

all: build

clean:
	rm -rf build

build: clean
	make -C assets install
	go build -o $(DESTDIR)/caronade ./cmd/caronade/...

.PHONY: all clean build
