DESTDIR?=	../build

all: build

caronade.css:
	sassc caronade.sass $@

caronade.min.css: caronade.css
	minify -o $@ caronade.css

clean:
	rm -f caronade.min.css caronade.css

install: caronade.min.css
	mkdir -p $(DESTDIR)
	mkdir -p $(DESTDIR)/builds
	cp -pr static templates work $(DESTDIR)/
	cp -p caronade.min.css $(DESTDIR)/static/
	cp -p caronade.css $(DESTDIR)/static/

.PHONY: all clean install
