# Simple Makefile for common dev tasks
.PHONY: build docker run

build:
	go build ./...

tidy:
	go mod tidy

docker:
	docker build -t trinity-cache:dev .

run:
	./cmd/trinity-cache/trinity-cache --version || true
