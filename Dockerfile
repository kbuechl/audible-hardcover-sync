FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-build
WORKDIR /src/internal/webui/frontend
COPY internal/webui/frontend/package.json internal/webui/frontend/package-lock.json ./
RUN npm ci
COPY internal/webui/frontend .
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party/go-audible/go.mod third_party/go-audible/go.mod
RUN go mod download
COPY . .
COPY --from=frontend-build /src/internal/webui/frontend/dist internal/webui/frontend/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/sync-tool ./cmd/sync-tool \
    && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/sync-tool /usr/local/bin/sync-tool
# The distroless image has no shell to create it, so copy in an empty /data.
COPY --from=build /out/data /data
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/sync-tool"]
CMD ["serve"]
