all: build

build:
	cp ../scripts/gen_kubeconfig.sh ./pkg/cmd/scripts/gen_kubeconfig.sh
	cp ../scripts/config-placeholder ./pkg/cmd/scripts/config-placeholder
	go build ./cmd/repctl

test:
	cp ../scripts/gen_kubeconfig.sh ./pkg/cmd/scripts/gen_kubeconfig.sh
	cp ../scripts/config-placeholder ./pkg/cmd/scripts/config-placeholder
	go clean -cache; cd ./pkg; go test -race -cover -coverprofile=coverage.out -coverpkg ./... ./...

# Install Go tools to build the code
tools:
	go list -f '{{range .Imports}}{{.}} {{end}}' pkg/tools/tools.go | xargs go install

mocks:
	mockery --all --disable-version-string

.PHONY: mocks
