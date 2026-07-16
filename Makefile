.PHONY: build run clean xai-login

build:
	go build -o openpaw .

run:
	go run .

# Obtain xAI Grok OAuth tokens from a SuperGrok / X Premium+ subscription.
# Writes ./xai_oauth.json, which OpenPaw registers as the `xai-oauth` provider.
# Add ARGS="-device" for headless boxes, or ARGS="-out data/xai_oauth.json".
xai-login:
	go run ./cmd/xai-login $(ARGS)

clean:
	rm -f openpaw
	rm -rf data/
