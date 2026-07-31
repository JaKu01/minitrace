FROM node:22-alpine AS app-build

WORKDIR /src/app
COPY app/package.json app/package-lock.json ./
RUN npm ci
COPY app/ ./
RUN npm run build

FROM golang:1.25-alpine AS server-build

WORKDIR /src
COPY go.work ./
COPY client/go.mod ./client/go.mod
COPY server/go.mod server/go.sum ./server/
COPY client/ ./client/
COPY server/ ./server/
COPY --from=app-build /src/app/dist/minitrace-app/browser/ ./server/web/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /minitrace ./server/cmd/minitrace

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 minitrace \
    && adduser -S -D -H -u 10001 -G minitrace minitrace \
    && mkdir -p /data \
    && chown 10001:10001 /data

COPY --from=server-build /minitrace /usr/local/bin/minitrace

USER 10001:10001
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/minitrace"]
CMD ["--database", "/data/minitrace.db", "--retention", "24h"]
