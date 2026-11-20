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
	@echo "Skipping checks on Windows, currently unsupported."
else
	@wget -O lint-project.sh https://raw.githubusercontent.com/moov-io/infra/master/go/lint-project.sh
	@chmod +x ./lint-project.sh
	COVER_THRESHOLD=70.0 EXPERIMENTAL=nilaway ./lint-project.sh
endif

# Extra utilities not needed for building

.PHONY: test
test:
	go test -cover github.com/moov-io/go-sftp/...

.PHONY: clean
clean:
ifeq ($(OS),Windows_NT)
	@echo "Skipping cleanup on Windows, currently unsupported."
else
	@rm -rf ./bin/ cover.out coverage.txt misspell* staticcheck*
endif

setup:
	docker compose up -d --force-recreate --remove-orphans
	$(MAKE) wait-sftp

.PHONY: wait-sftp
wait-sftp:
	@echo "waiting for sftp on :2222"
	@i=0; \
	while [ $$i -lt 45 ]; do \
		nc -z 127.0.0.1 2222 >/dev/null 2>&1 && echo "sftp is up" && exit 0; \
		i=$$((i+1)); sleep 1; \
	done; \
	echo "sftp did not become ready"; docker compose ps; docker compose logs; exit 1

.PHONY: test-sftp
test-sftp:
	go test -race -count=1 -timeout 15m ./...

teardown:
	docker compose down --remove-orphans

# For open source projects

# From https://github.com/genuinetools/img
.PHONY: AUTHORS
AUTHORS:
	@$(file >$@,# This file lists all individuals having contributed content to the repository.)
	@$(file >>$@,# For how it is generated, see `make AUTHORS`.)
	@echo "$(shell git log --format='\n%aN <%aE>' | LC_ALL=C.UTF-8 sort -uf)" >> $@
