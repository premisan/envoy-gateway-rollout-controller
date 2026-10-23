.PHONY: all build build-ui build-go test docker-build clean run

BINARY_NAME=bin/controller
IMAGE_TAG=premisan0/envoy-gateway-rollout-controller:latest

all: build

build-ui:
	cd ui && npm install && npm run build

build-go:
	CGO_ENABLED=0 go build -o $(BINARY_NAME) ./cmd/controller

build: build-ui build-go

test:
	go test -v ./pkg/...

docker-build:
	docker build -t $(IMAGE_TAG) .

docker-push:
	docker push $(IMAGE_TAG)

clean:
	rm -rf bin/ ui/dist/

run: build-ui
	go run ./cmd/controller --web-bind-address=:8080
