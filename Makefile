.PHONY: test build install

PREFIX ?= $(HOME)/.local
BIN := $(PREFIX)/bin/qianji

test:
	go test ./...

build:
	go build -o qianji ./cmd/qianji

install:
	sh tools/install.sh
