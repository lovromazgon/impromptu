.PHONY: build
build:
	go build -o impromptu cmd/impromptu/main.go

.PHONY: test
test:
	go test -v ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: clean
clean:
	rm -rf .impromptu_data
	rm -f impromptu
