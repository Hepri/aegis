.PHONY: all server client client-windows clean test help

BINARY_SERVER := aegis-server
BINARY_CLIENT := aegis-client.exe

all: server client-windows

server:
	go build -o $(BINARY_SERVER) ./cmd/aegis-server

client:
	go build -o $(BINARY_CLIENT) ./cmd/aegis-client

client-windows:
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_CLIENT) ./cmd/aegis-client

test:
	go test ./...

clean:
	rm -f $(BINARY_SERVER) $(BINARY_CLIENT)

help:
	@echo "targets:"
	@echo "  all, server, client-windows  - build binaries"
	@echo "  client                       - build client for current OS"
	@echo "  test                         - run tests"
	@echo "  clean                        - remove binaries"
