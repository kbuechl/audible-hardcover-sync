# Dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party/go-audible/go.mod third_party/go-audible/go.mod
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/sync-tool ./cmd/sync-tool

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/sync-tool /usr/local/bin/sync-tool
ENTRYPOINT ["/usr/local/bin/sync-tool"]
CMD ["serve"]
