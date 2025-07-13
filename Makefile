# Makefile for proktree

all: build

build:
	go build -o proktree -v

prodbuild:
	go build -ldflags "-s -w" -o proktree -v

test:
	go test -v .

install:
	go install

clean:
	go clean
	rm -f proktree
	rm -fr proktree-*

run:
	go build -o proktree -v
	./proktree

deps:
	go mod download
	go mod tidy

# Cross compilation
build-linux:
	GOOS=linux GOARCH=amd64 go build -o proktree-linux-amd64 -v

build-darwin:
	GOOS=darwin GOARCH=amd64 go build -o proktree-darwin-amd64 -v
	GOOS=darwin GOARCH=arm64 go build -o proktree-darwin-arm64 -v

build-all: build-linux build-darwin

.PHONY: all build prodbuild test install clean run deps build-linux build-darwin build-all test-linux
