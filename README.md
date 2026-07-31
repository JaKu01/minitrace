# minitrace

Small distributed tracing for Go services: one client package, one server,
one built-in dashboard. Traces are stored in SQLite for at most 24 hours.

## Layout

```text
minitrace/
├── client/  Go instrumentation package
├── server/  ingestion API, query API and SQLite storage
└── app/     Angular dashboard
```

## Run locally for development

Requirements: Go 1.25+, Node.js 20.19+ (Node 22 is recommended).

Build the Angular app:

```bash
cd app
npm install
npm run build
```

Start the trace server from the repository root:

```bash
go run ./server/cmd/minitrace \
  --database ./minitrace.db \
  --app-dir ./app/dist/minitrace-app/browser
```

Open [http://localhost:8080](http://localhost:8080). During frontend
development, `npm start` proxies `/api` to the server automatically.

The release image builds Angular first and embeds the resulting files into the
Go binary. The deployed container therefore contains one executable and needs
no Node.js runtime or separate web directory.

```bash
docker build -t minitrace .
docker run --rm -p 8080:8080 -v minitrace-data:/data minitrace
```

Retention defaults to 24 hours and is capped at 24 hours. A shorter period can
be selected:

```bash
go run ./server/cmd/minitrace --retention 6h
```

## Instrument a Go service

```go
package main

import (
	"context"
	"net/http"
	"time"

	minitrace "github.com/JaKu01/minitrace/client"
)

func main() {
	tracer, err := minitrace.New(minitrace.Config{
		ServiceName: "checkout",
		Endpoint:    "http://localhost:8080",
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tracer.Close(ctx)
	}()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "load-cart")
		defer span.End()

		_ = ctx // pass ctx to nested calls
		span.SetAttribute("cart.id", "cart-42")
		w.WriteHeader(http.StatusNoContent)
	})

	_ = http.ListenAndServe(":3000", tracer.Middleware(handler))
}
```

`Span.End` only writes to a bounded in-memory queue. A background exporter
sends batches to `POST /api/v1/spans`; a full queue drops spans instead of
blocking the application. `Tracer.Transport` creates spans for outgoing HTTP
calls and propagates W3C `traceparent` headers to downstream services.

## Kubernetes deployment with Pulumi

The Pulumi project builds and pushes `registry.kurth.dev/minitrace:latest`,
creates a single-replica deployment, a `2Gi` persistent volume for SQLite, an
internal service named `minitrace`, and an authenticated Traefik route. The
default dashboard hostname is `traces.kurth.dev`.

```bash
cd pulumi
npm install
pulumi stack init prod
pulumi config set kubernetes:context YOUR_CONTEXT
pulumi config set hostname traces.kurth.dev
pulumi up
```

Optional settings are `namespace`, `imageName`, `storageSize`, and
`retentionHours`. Retention values above 24 hours are capped in Pulumi and once
again by the server. Applications inside the cluster export to:

```text
http://minitrace:8080
```

## Server API

- `POST /api/v1/spans` — ingest up to 1,000 spans per batch
- `GET /api/v1/traces` — list traces, with `service`, `q`, `status`, and `limit`
- `GET /api/v1/traces/{traceId}` — load the full waterfall
- `GET /api/v1/services` — list active services
- `GET /api/v1/health` — health and effective retention
