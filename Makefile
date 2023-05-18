
.PHONY: $(shell ls)

BASE_IMAGE = amd64/golang:1.14-alpine3.11
GOLANGCI_VERSION = 1.52.2


PATH := $(PWD)/bin:$(PATH)

help:
	@echo "usage: make [action]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy       run go mod tidy"
	@echo "  format         format source files"
	@echo "  test           run available tests"
	@echo "  run ARGS=args  run app"
	@echo "  release        build release assets"
	@echo "  travis-setup   setup travis CI"
	@echo ""

blank :=
define NL

$(blank)
endef

mod-tidy:
	docker run --rm -it -v $(PWD):/s $(BASE_IMAGE) \
	sh -c "apk add git && cd /s && GOPROXY=direct go get && GOPROXY=direct go mod tidy"

format:
	docker run --rm -it -v $(PWD):/s $(BASE_IMAGE) \
	sh -c "cd /s && find . -type f -name '*.go' | xargs gofmt -l -w -s"

define DOCKERFILE_TEST
FROM $(BASE_IMAGE)
RUN apk add --no-cache make docker-cli git
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
endef
export DOCKERFILE_TEST

test:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t temp
	docker run --rm -it \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	temp \
	make test-nodocker

test-nodocker:
	$(foreach IMG,$(shell echo test-images/*/ | xargs -n1 basename), \
	docker build -q test-images/$(IMG) -t rtp-proxy-test-$(IMG)$(NL))
	$(eval export CGO_ENABLED = 0)
	go test -v .

define DOCKERFILE_RUN
FROM $(BASE_IMAGE)
RUN apk add --no-cache git
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN GOPROXY=direct go build -o /out .
endef
export DOCKERFILE_RUN

define CONFFILE
server:
  protocols: [ tcp, udp ]
  rtspPort: 8555
  rtpPort: 8050
  rtcpPort: 8051

streams:
  test1:
    url: rtsp://localhost:8554/mystream

endef
export CONFFILE

ARGS ?= stdin

run:
	echo "$$DOCKERFILE_RUN" | docker build -q . -f - -t temp
	echo "$$CONFFILE" | docker run --rm -i \
	--network=host \
	temp \
	/out $(ARGS)

define DOCKERFILE_RELEASE
FROM $(BASE_IMAGE)
RUN apk add --no-cache zip make git tar
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN make release-nodocker
endef
export DOCKERFILE_RELEASE

release:
	echo "$$DOCKERFILE_RELEASE" | docker build . -f - -t temp \
	&& docker run --rm -it -v $(PWD):/out \
	temp sh -c "rm -rf /out/release && cp -r /s/release /out/"

release-nodocker:
	$(eval VERSION := $(shell git describe --tags))
	$(eval GOBUILD := go build -ldflags '-X "main.Version=$(VERSION)"')
	rm -rf release && mkdir release

	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -o /tmp/rtsp-simple-proxy.exe
	cd /tmp && zip -q $(PWD)/release/rtsp-simple-proxy_$(VERSION)_windows_amd64.zip rtsp-simple-proxy.exe

	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o /tmp/rtsp-simple-proxy
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-proxy_$(VERSION)_linux_amd64.tar.gz --owner=0 --group=0 rtsp-simple-proxy

	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 $(GOBUILD) -o /tmp/rtsp-simple-proxy
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-proxy_$(VERSION)_linux_arm6.tar.gz --owner=0 --group=0 rtsp-simple-proxy

	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GOBUILD) -o /tmp/rtsp-simple-proxy
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-proxy_$(VERSION)_linux_arm7.tar.gz --owner=0 --group=0 rtsp-simple-proxy

	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -o /tmp/rtsp-simple-proxy
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-proxy_$(VERSION)_linux_arm64.tar.gz --owner=0 --group=0 rtsp-simple-proxy

	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -o /tmp/rtsp-simple-proxy
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-proxy_$(VERSION)_darwin_amd64.tar.gz --owner=0 --group=0 rtsp-simple-proxy

define DOCKERFILE_TRAVIS
FROM ruby:alpine
RUN apk add --no-cache build-base git
RUN gem install travis
endef
export DOCKERFILE_TRAVIS

travis-setup:
	echo "$$DOCKERFILE_TRAVIS" | docker build - -t temp
	docker run --rm -it \
	-v $(PWD):/s \
	temp \
	sh -c "cd /s \
	&& travis setup releases --force"

# Generate code
# Requires protoc be installed
generate: download-deps
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/v1alpha1/msm_dp/*.proto	

# Download build dependencies
download-deps:
	./scripts/download-deps.sh

check: fumpt vet lint ## Run tests and linters

bin/golangci-lint: bin/golangci-lint-${GOLANGCI_VERSION}
	@ln -sf golangci-lint-${GOLANGCI_VERSION} bin/golangci-lint
bin/golangci-lint-${GOLANGCI_VERSION}:
	@mkdir -p bin
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | bash -s -- -b ./bin v${GOLANGCI_VERSION}
	@mv bin/golangci-lint $@

.PHONY: lint
lint: bin/golangci-lint ## Run linter
	bin/golangci-lint run -c .golangci.yaml --timeout 3m

# Run go fmt against code
fmt:
	go fmt ./...

# Run go fumpt against code
fumpt:
	gofumpt -d -w .

# Run go vet against code
vet:
	go vet ./...
