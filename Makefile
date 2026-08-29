.PHONY: up down api mobile generate check clean

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

check:         ## what we run before looking at your submission
	cd api && go build ./... && go vet ./... && go test ./...
	cd mobile && npx tsc --noEmit

clean:
	docker compose down -v
