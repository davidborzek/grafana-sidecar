FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /grafana-sidecar ./cmd/grafana-sidecar

FROM gcr.io/distroless/static:nonroot
COPY --from=build /grafana-sidecar /grafana-sidecar
USER nonroot:nonroot
ENTRYPOINT ["/grafana-sidecar"]
