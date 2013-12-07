build:
	go build -o shadow
	sass --scss static/style.scss static/style.css
	coffee -c static/main.coffee

tarball: build
	tar czvf shadow.tgz shadow static
