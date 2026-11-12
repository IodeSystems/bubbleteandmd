.PHONY: demo

# demo builds the split-screen showcase (cmd/demo) to ./bin/demo.
demo:
	go build -o bin/demo ./cmd/demo
