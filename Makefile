.PHONY: build run clean

build:
	go build -o openpaw .

run:
	go run .

clean:
	rm -f openpaw
	rm -rf data/
