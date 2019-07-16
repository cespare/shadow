.PHONY: build

build:
	sass static/style.scss static/style.css
	coffee -c static/main.coffee
	go build -o shadow
