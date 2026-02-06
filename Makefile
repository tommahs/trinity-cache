# Simple Makefile for common dev tasks
.PHONY: build tidy docker run clean

bin := ./bin

build:
	mkdir -p $(bin)
	go build -o $(bin)/trinity-cache ./cmd/trinity-cache


tidy:
	go mod tidy


docker:
	docker build -t trinity-cache:dev .

run: build
	$(bin)/trinity-cache --version || true

clean:
	rm -rf $(bin)
