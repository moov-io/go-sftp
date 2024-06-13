# generated-from:24ff9e514b3feb9cead4ac1040b8cc9ada535b9d8ee3cfd2f65f4f95acd234f0 DO NOT REMOVE, DO UPDATE

PLATFORM=$(shell uname -s | tr '[:upper:]' '[:lower:]')
PWD := $(shell pwd)

ifndef VERSION
	VERSION := $(shell git describe --tags --abbrev=0)
endif

COMMIT_HASH :=$(shell git rev-parse --short HEAD)
DEV_VERSION := dev-${COMMIT_HASH}

USERID := $(shell id -u $$USER)
GROUPID:= $(shell id -g $$USER)

export GOPRIVATE=github.com/moovfinancial

all: check

.PHONY: check
check:
ifeq ($(OS),Windows_NT)
	go test ./...
else
	@wget -O lint-project.sh https://raw.githubusercontent.com/moov-io/infra/master/go/lint-project.sh
	@chmod +x ./lint-project.sh
	COVER_THRESHOLD=70.0 ./lint-project.sh
endif

# Extra utilities not needed for building

test: update
	go test -cover github.com/moov-io/go-sftp/...

.PHONY: clean
clean:
ifeq ($(OS),Windows_NT)
	@echo "Skipping cleanup on Windows, currently unsupported."
else
	@rm -rf ./bin/ cover.out coverage.txt misspell* staticcheck*
endif

PROFILE ?= unix
ifeq ($(OS),Windows_NT)
	PROFILE := windows
endif

setup:
	docker-compose --profile $(PROFILE) up -d --force-recreate --remove-orphans sftp-$(PROFILE)

teardown:
	docker-compose --profile $(PROFILE) down --remove-orphans

# For open source projects

# From https://github.com/genuinetools/img
.PHONY: AUTHORS
AUTHORS:
	@$(file >$@,# This file lists all individuals having contributed content to the repository.)
	@$(file >>$@,# For how it is generated, see `make AUTHORS`.)
	@echo "$(shell git log --format='\n%aN <%aE>' | LC_ALL=C.UTF-8 sort -uf)" >> $@
