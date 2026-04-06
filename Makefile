CGO_CFLAGS := -I$(CURDIR)/include
CGO_LDFLAGS := $(CURDIR)/lib/darwin_arm64/liblancedb_go.a -framework Security -framework CoreFoundation

export CGO_CFLAGS
export CGO_LDFLAGS

.PHONY: build run clean

build:
	go build -o openpaw .

run:
	go run .

clean:
	rm -f openpaw
	rm -rf data/
