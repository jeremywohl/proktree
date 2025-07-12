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

run:
	go build -o proktree -v
	./proktree

deps:
	go mod download
	go mod tidy

.PHONY: all build prodbuild test install clean run deps
