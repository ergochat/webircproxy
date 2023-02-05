.PHONY: build install test gofmt bench

GIT_COMMIT := $(shell git rev-parse HEAD 2> /dev/null)

# disable linking against native libc / libpthread by default;
# this can be overridden by passing CGO_ENABLED=1 to make
export CGO_ENABLED ?= 0

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

bench:
	cd irc && go test -benchmem -bench .
