all: main.min.css

main.css:
	sassc sass/main.sass main.css

main.min.css: main.css
	minify -o $@ main.css

clean:
	rm -f main.min.css main.css

install: normalize.min.css milligram.min.css main.min.css
	cat normalize.min.css > ../static/caronade.min.css
	cat milligram.min.css >> ../static/caronade.min.css
	cat main.min.css >> ../static/caronade.min.css

.PHONY: clean install
