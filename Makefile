.PHONY: all install

all: install

install:
	CGO_ENABLED=0 go install -trimpath -ldflags "-w -s" .

TEST_ARGS?=
TEST_PACKAGE?=./...

COVERAGE_OUT?=cover.out
COVERAGE_HTML?=coverage.html

COVERAGE_ARGS=-cover -covermode atomic -coverprofile $(COVERAGE_OUT)

.PHONY: test testcover coverage cover

test:
	go test -trimpath -ldflags "-w -s" -race $(TEST_ARGS) $(TEST_PACKAGE)

testcover:
	go test -trimpath -ldflags "-w -s" -race $(COVERAGE_ARGS) $(TEST_ARGS) $(TEST_PACKAGE)

coverage:
	go tool cover -html $(COVERAGE_OUT) -o $(COVERAGE_HTML)

cover: testcover coverage ## Test with coverage

.PHONY: fmt vet prepare

fmt:
	goimports -w .

vet:
	go vet ./...

prepare: fmt vet

DOCKER_NAME=xorkevin/anvil

DOCKER_MAJOR_VERSION=0.1
DOCKER_VERSION=0.1.0

DOCKER_LATEST_IMAGE=$(DOCKER_NAME):latest
DOCKER_MAJOR_IMAGE=$(DOCKER_NAME):$(DOCKER_MAJOR_VERSION)
DOCKER_IMAGE=$(DOCKER_NAME):$(DOCKER_VERSION)

.PHONY: build-docker publish-docker docker

build-docker:
	docker build -t $(DOCKER_IMAGE) -t $(DOCKER_MAJOR_IMAGE) -t $(DOCKER_LATEST_IMAGE) .

publish-docker:
	docker push $(DOCKER_IMAGE)
	docker push $(DOCKER_MAJOR_IMAGE)
	docker push $(DOCKER_LATEST_IMAGE)

docker: build-docker publish-docker
