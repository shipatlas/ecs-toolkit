SHELL := /bin/bash

BIN_DIR := ./bin

all: dependencies build

dependencies:
	go mod vendor

build:
	@mkdir -p ${BIN_DIR}
	go build -o ${BIN_DIR}

clean:
	rm -rf ${BIN_DIR}

lint:
	golangci-lint run

test:
	go test -v ./...
