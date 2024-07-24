all: build

build:
	cp -f ../scripts/gen_kubeconfig.sh ./pkg/cmd/scripts/gen_kubeconfig.sh
	cp -f ../scripts/config-placeholder ./pkg/cmd/scripts/config-placeholder
	go build ./cmd/repctl

# Build static binary, the build can be platform dependent so may need tweaking.
build-static:
	cp -f ../scripts/gen_kubeconfig.sh ./pkg/cmd/scripts/gen_kubeconfig.sh
	cp -f ../scripts/config-placeholder ./pkg/cmd/scripts/config-placeholder
	CGO_ENABLED=1 CGO_LDFLAGS="-static" go build ./cmd/repctl

test:
	cp ../scripts/gen_kubeconfig.sh ./pkg/cmd/scripts/gen_kubeconfig.sh
	cp ../scripts/config-placeholder ./pkg/cmd/scripts/config-placeholder
	go clean -cache; cd ./pkg; go test -race -cover -coverprofile=coverage.out -coverpkg ./... ./...

# Install Go tools to build the code
tools:
	go list -f '{{range .Imports}}{{.}} {{end}}' pkg/tools/tools.go | xargs go install

mocks:
	mockery --all --disable-version-string

clean:
	rm -f ./pkg/cmd/scripts/gen_kubeconfig.sh
	rm -f ./pkg/cmd/scripts/config-placeholder
	go clean -cache
	rm -f repctl

.PHONY: mocks
