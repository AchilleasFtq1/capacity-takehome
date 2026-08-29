# Capacity — take-home scaffold

**Read [BRIEF.md](BRIEF.md) first.** That is the actual exercise. This file just
gets you running.

Everything here builds, boots and talks to a database already. You should not
have to fight the toolchain — if you do, tell us, that's a bug in our scaffold
and not a test.

## Run it

Needs Go 1.25+, Node 20+, Docker.

```bash
make up        # mongo on :27117, as a single-node replica set
make api       # graphql on :8080, seeds ten users on first boot
make mobile    # expo; press i for the iOS simulator, w for web
```

Then open <http://localhost:8080> for the GraphQL playground.

```bash
make check     # go build + vet + test, and tsc on the client
make down      # stop mongo (keeps data)
make clean     # stop mongo and delete data
```

## Prove it works before you start

```bash
curl -s localhost:8080/query -H 'Content-Type: application/json' \
  -d '{"query":"{ users { id name } }"}'
```

Ten users come back. Copy any `id` and send it as the `X-User-Id` header — that
is the whole authentication story and it is intentionally fake:

```bash
curl -s localhost:8080/query -H 'Content-Type: application/json' \
  -H 'X-User-Id: <paste an id>' \
  -d '{"query":"{ me { name } }"}'
```

## What's here

```
api/
  graph/                    gqlgen schema + generated code + resolvers
    schema.graphqls         the API surface. edit it, then `make generate`
    schema.resolvers.go     me and users work; everything else panics. that's yours
  internal/
    capacity/               THE GRADED SURFACE. pure rules, no IO. stubs + test stubs
    config/                 caps loaded from env. already done, don't undo it
    store/                  mongo: documents, connection, indexes, seed
  cmd/server/main.go        wiring, playground, the X-User-Id middleware
mobile/
  App.tsx                   a user switcher, so you can act as anyone. replace it
  src/api.ts                a 20-line graphql fetch. swap it if you'd rather
```

## Where to start

`api/internal/capacity/capacity.go` has three functions returning
`errNotImplemented`, and a test file with five `t.Skip`ed tests naming the rules
they should prove. Delete a Skip, write the test, make it pass. The concurrency
rule needs a real database, so its stub lives in `api/internal/store/race_test.go`.

Then work outwards: resolvers in `graph/schema.resolvers.go`, screens in `mobile/`.

## Things worth knowing

- **Mongo runs as a replica set, not a standalone.** That is deliberate:
  transactions are available to you. Whether you need them is your call.
- **Mongo is on 27117**, not the default 27017, so it can't collide with a Mongo
  you already have running.
- Changing `schema.graphqls` means running `make generate`. gqlgen preserves
  resolver bodies you've already written.
- On a physical device `localhost` won't reach the API. Set
  `EXPO_PUBLIC_API_URL=http://<your-lan-ip>:8080/query`.
- Caps are env vars: `CAP_BUDGET`, `CAP_PARTNER`, `CAP_CREW`, `CAP_CIRCLE`.
  Try `CAP_CIRCLE=500 make api` — nothing should need recompiling for that to
  take effect, and nothing should break.

## Handing it back

Replace this README with your own. Keep the run instructions, and add the four
sections BRIEF.md asks for: decisions, what's next, what's unfinished, and where
your agent got something wrong.
