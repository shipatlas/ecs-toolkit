SHELL := /bin/bash

BIN_DIR := ./bin
PACKAGE_PATH := $(shell head -n1 go.mod | cut -d' ' -f2)
TAG_VERSION := $(tag)

LDFLAGS := -X $(PACKAGE_PATH)/cmd.versionTag=$(TAG_VERSION)

all: dependencies build

dependencies:
	go mod vendor

build:
	@mkdir -p ${BIN_DIR}
	go build -ldflags="$(LDFLAGS)" -o ${BIN_DIR}
	@echo
	@${BIN_DIR}/ecs-toolkit version

clean:
	rm -rf ${BIN_DIR}

lint:
	golangci-lint run

test:
	go test -v ./...
