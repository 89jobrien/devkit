build:
    go build -o devkit ./cmd/devkit

council base="HEAD~10":
    op run --account=my.1password.com --env-file=$HOME/.secrets -- devkit council --base {{base}}

install:
    go install ./cmd/devkit

test:
    go test ./...

lint:
    go vet ./...
