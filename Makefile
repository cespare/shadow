.PHONY: build

build:
	sass --scss static/style.scss static/style.css
	coffee -c static/main.coffee
	go build -o shadow
