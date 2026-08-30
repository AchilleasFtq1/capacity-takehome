.PHONY: up down api mobile generate check test-rules clean

up:            ## start mongo (single-node replica set, so transactions work)
	docker compose up -d
	@echo "waiting for mongo to report healthy..."
	@until [ "$$(docker inspect -f '{{.State.Health.Status}}' capacity-mongo 2>/dev/null)" = "healthy" ]; do sleep 2; done
	@echo "mongo ready on :27117"

down:
	docker compose down

api:           ## run the graphql api on :8080
	cd api && go run ./cmd/server

mobile:        ## run the expo client
	cd mobile && npm start

generate:      ## regenerate gqlgen code after editing graph/schema.graphqls
	cd api && go run github.com/99designs/gqlgen generate

# `check` now depends on `up`. Rule 4 - two accepts racing for the last seat -
# can only be proven against a real replica set, and its test skips itself when
# Mongo is unreachable. Skipping it quietly would mean the one rule most likely
# to be got wrong is also the one most likely to go unchecked.
check: up      ## what we run before looking at your submission
	cd api && go build ./... && go vet ./... && go test ./...
	cd mobile && npm install --silent && npx tsc --noEmit

test-rules: up ## just the four rules, verbosely, with the race detector
	cd api && go test ./internal/capacity/ ./internal/store/ -race -count=1 -v

clean:
	docker compose down -v
