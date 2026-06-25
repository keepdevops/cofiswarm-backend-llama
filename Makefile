ROLE := backend-llama
.PHONY: test test-standalone-layout test-go
test: test-standalone-layout test-go
test-standalone-layout:
	./test/scripts/assert-layout.sh $(ROLE)
test-go:
	go test ./...
