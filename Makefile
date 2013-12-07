build:
	sass --scss static/style.scss static/style.css
	coffee -c static/main.coffee

tarball:
	go build -o shadow
	tar czvf shadow.tgz shadow static
