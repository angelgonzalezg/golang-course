# MongoDB document lab

Learn document persistence in an independent Go 1.27 module using the official
MongoDB Go driver v2.10.0. The first stage runs real CRUD against MongoDB 8.0.38,
compares BSON presence with SQL NULL, and checks indexes, context, and cleanup.

| Stage | Status |
| --- | --- |
| 1. Fundamentals | Implemented: [complete walkthrough and standalone execution/test guide](01-fundamentals/README.md), ObjectID/BSON, CRUD, bounded cursors, field updates, unique indexes, validation, and Testcontainers tests. |
| 2. Document modeling and queries | Implemented: [standalone embedded-document/query guide](02-document-modeling/README.md), address/tags modeling, embedding versus reference decisions, city/tag filters, projected summaries, ordered limits, and nested updates with real MongoDB tests. |
| Further queries and schema evolution | Proposed: cross-collection references/aggregation, query-plan exploration, and explicit schema evolution. |

## Run directly

Requirements: Go 1.27 on PATH, a full checkout, and a running Docker-compatible
engine with Docker CLI/Compose v2. Run bash/zsh from the repository root containing
`go.mod`, `chapters`, and `extensions`. First runs download pinned modules/images.

```sh
go version
docker info
docker compose version
docker compose -f extensions/03-mongodb/compose.yaml up -d --wait
export MONGODB_URI='mongodb://mongo_lab:mongo_lab_local@127.0.0.1:27018/mongo_lab?authSource=mongo_lab'
go -C extensions/03-mongodb run ./01-fundamentals
```

Compose exposes the real server on `127.0.0.1:27018` and retains a named volume.
Its initialization script gives the application user read/write access to the
`mongo_lab` database. These local teaching credentials use loopback without TLS.
The program explicitly selects `mongo_lab.contacts`, bootstraps its unique name
index, runs five finite sections, and attempts to remove only its own document.
Look for `Duplicate name -> ErrDuplicate`, `Data belongs to MongoDB`, and
`repeated lookup -> ErrNotFound`. The longer stage guide explains every operation.

Using the same database and `MONGODB_URI`, run the second finite lesson directly:

```sh
go -C extensions/03-mongodb run ./02-document-modeling
```

It uses `mongo_lab.contact_profiles`, embeds an address and tags, combines city/tag
filters, projects summaries, and updates only `address.city`. Look for
`street, name, and tags stayed intact`. It attempts to remove only its own three
profiles. Its [README](02-document-modeling/README.md) also includes the complete
setup and validation steps for starting directly at stage 2.

After the program exits, stop the manual database:

```sh
docker compose -f extensions/03-mongodb/compose.yaml down
```

The volume remains. `down -v` intentionally discards this lab's MongoDB data and
users. Initialization scripts run only on an empty volume; changing Compose
credentials later does not change existing database users.

## Verify directly

Ordinary tests cover input/query validation, BSON encoding/decoding, slice ownership,
and error causes.
They need no running database. Race checks additionally need clang or gcc.

```sh
go -C extensions/03-mongodb test -count=1 -timeout=60s ./...
go -C extensions/03-mongodb vet ./...
go -C extensions/03-mongodb mod tidy -diff
CGO_ENABLED=1 go -C extensions/03-mongodb test -race -count=1 -timeout=90s ./...
```

Tagged integration tests own a real, authenticated MongoDB container. They wait
for readiness and clean it up, without using Compose or `MONGODB_URI`. Start the
engine; a missing runtime makes these tests fail. The first run may pull images.

For Rancher Desktop on macOS, select **dockerd (moby)**. With the home-directory
socket and ports forwarded to localhost, set these in the test terminal:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually uses automatic detection. If Rancher exposes
ports through a VM address, use that reachable address instead; inspect it with
`rdctl shell ip a show vznat`. Keep the host override nonempty. With a C compiler:

```sh
docker info
CGO_ENABLED=1 go -C extensions/03-mongodb test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/03-mongodb vet -tags=integration ./...
```

Expect `ok` for tested packages, `[no test files]` for command/helper packages,
and quiet output from successful vet/metadata checks. CI includes this module
in ordinary/race checks and runs the MongoDB integration suite separately.

After the first run, follow the [fundamentals guide](01-fundamentals/README.md),
then [document modeling and queries](02-document-modeling/README.md) to read the
commented implementation and tests. Further aggregation/schema evolution and
caching remain separate blocks; guided exercises remain the final course block.
