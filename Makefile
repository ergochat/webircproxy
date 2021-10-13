.PHONY: build install test gofmt

GIT_COMMIT := $(shell git rev-parse HEAD 2> /dev/null)

build:
	go build -v -ldflags "-X main.commit=$(GIT_COMMIT)"

install:
	go install -v -ldflags "-X main.commit=$(GIT_COMMIT)"

#release:
#	goreleaser --skip-publish --rm-dist

test:
	cd irc && go test . && go vet .
	./.check-gofmt.sh

gofmt:
	./.check-gofmt.sh --fix
