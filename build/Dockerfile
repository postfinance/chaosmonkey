# renovate: datasource=docker packageName=golang
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY pkg/ pkg/
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /chaosmonkey ./cmd/chaosmonkey

FROM scratch
COPY --from=build /chaosmonkey /chaosmonkey
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
USER 65534:65534
ENTRYPOINT ["/chaosmonkey"]
