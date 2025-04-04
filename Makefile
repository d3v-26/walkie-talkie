BINARY_NAME = walkietalkie
GO_FILES = main.go

build: build-native

build-native:
	mkdir -p build/native
	go build -o build/native/$(BINARY_NAME) $(GO_FILES)

build-linux:
	mkdir -p build/linux
	GOOS=linux GOARCH=amd64 go build -o build/linux/$(BINARY_NAME) $(GO_FILES)

build-windows:
	mkdir -p build/microsoft
	GOOS=windows GOARCH=amd64 go build -o build/microsoft/$(BINARY_NAME)-windows.exe $(GO_FILES)

build-darwin:
	mkdir -p build/darwin
	GOOS=darwin GOARCH=amd64 go build -o build/darwin/$(BINARY_NAME) $(GO_FILES)

clean:
	rm -rf build

.PHONY: build build-native build-linux build-windows build-darwin clean
