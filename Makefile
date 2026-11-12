.PHONY: demo install

# demo builds the split-screen showcase (cmd/demo) to ./bin/demo.
demo:
	go build -o bin/demo ./cmd/demo

# install puts the demo in $(go env GOBIN), else $(go env GOPATH)/bin.
install:
	go install ./cmd/demo
