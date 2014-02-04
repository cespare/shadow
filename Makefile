export GOPATH=$(PWD):$(PWD)/vendor

build: deps
	sass --scss static/style.scss static/style.css
	coffee --no-header -c static/main.coffee
	go build -o shadow

deps:
	git submodule update --init

tarball: build
	tar czvf shadow.tgz shadow static

clean:
	rm -fr shadow
