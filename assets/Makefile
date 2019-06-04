all: main.min.css

main.css:
	sassc bulma.sass $@

main.min.css: main.css
	minify -o $@ main.css

clean:
	rm -f main.min.css main.css

install:  main.min.css
	cp -p main.min.css ../static/caronade.min.css

.PHONY: all clean install
