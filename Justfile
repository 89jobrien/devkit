build:
    go build -o devkit ./cmd/devkit

install:
    go install ./cmd/devkit

test:
    go test ./...

lint:
    go vet ./...
