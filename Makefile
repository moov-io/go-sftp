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

all: install update build

.PHONY: install
install:
	go mod tidy
	go install github.com/markbates/pkger/cmd/pkger@latest
	go mod vendor

update:
	pkger -include /migrations -include /configs/config.default.yml
	go mod vendor

build:
	go build -mod=vendor -ldflags "-X github.com/moov-io/go-sftp.Version=${VERSION}" -o bin/metabank-reports github.com/moov-io/go-sftp/cmd/metabank-reports

.PHONY: setup
setup:
	docker-compose up -d --force-recreate --remove-orphans
	docker-compose exec -T vault bash -c "/vault-setup.sh -k 'card-number vpp-audit-log'"


.PHONY: check
check:
ifeq ($(OS),Windows_NT)
	@echo "Skipping checks on Windows, currently unsupported."
else
	@wget -O lint-project.sh https://raw.githubusercontent.com/moov-io/infra/master/go/lint-project.sh
	@chmod +x ./lint-project.sh
	./lint-project.sh # COVER_THRESHOLD=75.0
endif

.PHONY: teardown
teardown:
	-docker-compose down --remove-orphans

docker: update
	docker build --pull --build-arg VERSION=${VERSION} -t moov-io/go-sftp:${VERSION} -f Dockerfile .
	docker tag moov-io/go-sftp:${VERSION} moov-io/go-sftp:${VERSION}
	docker tag moov-io/go-sftp:${VERSION} moov-io/go-sftp:latest

docker-push:
ifeq ($(shell docker manifest inspect moov-io/go-sftp:${VERSION} > /dev/null ; echo $$?), 0)
	$(error docker tag already exists)
endif
	docker push moov-io/go-sftp:${VERSION}
	docker push moov-io/go-sftp:latest

.PHONY: dev-docker
dev-docker: update
	docker build --pull --build-arg VERSION=${DEV_VERSION} -t moov-io/go-sftp:${DEV_VERSION} -f Dockerfile .
	docker tag moov-io/go-sftp:${DEV_VERSION} moov-io/go-sftp:${DEV_VERSION}

PHONY: dev-push
dev-push:
	docker push moov-io/go-sftp:${DEV_VERSION}

# Extra utilities not needed for building

run: update build
	./bin/metabank-reports

docker-run:
	docker run -v ${PWD}/data:/data -v ${PWD}/configs:/configs --env APP_CONFIG="/configs/config.yml" -it --rm moov-io/go-sftp:${VERSION}

test: update
	go test -cover github.com/moov-io/go-sftp/...

.PHONY: clean
clean:
ifeq ($(OS),Windows_NT)
	@echo "Skipping cleanup on Windows, currently unsupported."
else
	@rm -rf cover.out coverage.txt misspell* staticcheck*
	@rm -rf ./bin/
endif

# For open source projects

# From https://github.com/genuinetools/img
.PHONY: AUTHORS
AUTHORS:
	@$(file >$@,# This file lists all individuals having contributed content to the repository.)
	@$(file >>$@,# For how it is generated, see `make AUTHORS`.)
	@echo "$(shell git log --format='\n%aN <%aE>' | LC_ALL=C.UTF-8 sort -uf)" >> $@

dist: clean build
ifeq ($(OS),Windows_NT)
	CGO_ENABLED=1 GOOS=windows go build -o bin/metabank-reports.exe cmd/metabank-reports/*
else
	CGO_ENABLED=1 GOOS=$(PLATFORM) go build -o bin/metabank-reports-$(PLATFORM)-amd64 cmd/metabank-reports/*
endif
