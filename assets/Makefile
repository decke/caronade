DESTDIR?=	../build

all: install

caronade.css:
	sassc caronade.sass $@

caronade.min.css:
	sassc -t compressed caronade.sass $@

clean:
	rm -f caronade.min.css caronade.css

install: caronade.css caronade.min.css
	mkdir -p $(DESTDIR)
	mkdir -p $(DESTDIR)/builds
	cp -pr static templates work $(DESTDIR)/
	cp -p caronade.min.css $(DESTDIR)/static/
	cp -p caronade.css $(DESTDIR)/static/

.PHONY: all clean install
