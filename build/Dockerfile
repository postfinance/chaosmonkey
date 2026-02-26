# renovate: datasource=docker packageName=golang
FROM docker.repo.pnet.ch/golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY pkg/ pkg/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /chaosmonkey ./cmd/chaosmonkey

FROM scratch
COPY --from=build /chaosmonkey /chaosmonkey
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
USER 65534:65534
ENTRYPOINT ["/chaosmonkey"]
