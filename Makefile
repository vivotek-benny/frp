export PATH := $(GOPATH)/bin:$(PATH)
export GO111MODULE=on
LDFLAGS := -s -w

all: fmt build

build: monitoragentc

# compile assets into binary file
file:
	rm -rf ./assets/monitoragentc/static/*
	cp -rf ./web/monitoragentc/dist/* ./assets/monitoragentc/static

fmt:
	go fmt ./...

fmt-more:
	gofumpt -l -w .

gci:
	gci write -s standard -s default -s "prefix(github.com/fatedier/frp/)" ./

vet:
	go vet ./...

monitoragentc:
	env CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -tags monitoragentc -o bin/monitoragentc ./cmd/monitoragentc

test: gotest

gotest:
	go test -v --cover ./assets/...
	go test -v --cover ./cmd/...
	go test -v --cover ./client/...
	go test -v --cover ./pkg/...

e2e:
	./hack/run-e2e.sh

e2e-trace:
	DEBUG=true LOG_LEVEL=trace ./hack/run-e2e.sh

e2e-compatibility-last-monitoragentc:
	if [ ! -d "./lastversion" ]; then \
		TARGET_DIRNAME=lastversion ./hack/download.sh; \
	fi
	monitoragentc_PATH="`pwd`/lastversion/monitoragentc" ./hack/run-e2e.sh
	rm -r ./lastversion

alltest: vet gotest e2e

clean:
	rm -f ./bin/monitoragentc
	rm -rf ./lastversion
