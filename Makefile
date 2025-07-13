# Makefile for proktree

PREFIX ?= /usr/local

all: build

build:
	go build -o proktree -v

prodbuild:
	go build -ldflags "-s -w" -o proktree -v

test:
	go test -v .

install:
	go install

# Install man page (defaults to /usr/local, override with PREFIX=/opt/local make install-man)
install-man:
	mkdir -p $(PREFIX)/share/man/man1
	cp proktree.1 $(PREFIX)/share/man/man1/
	gzip -f $(PREFIX)/share/man/man1/proktree.1

# Install both binary and man page
install-all: install install-man

clean:
	go clean
	rm -f proktree
	rm -fr proktree-*

fmt:
	go fmt .

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

.PHONY: all build prodbuild test install install-man install-all clean run deps build-linux build-darwin build-all
