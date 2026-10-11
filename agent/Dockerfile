FROM golang:1.24-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /lazy-pelican-agent ./cmd/lazy-pelican-agent

FROM gcr.io/distroless/static:latest
COPY --from=build /lazy-pelican-agent /usr/bin/lazy-pelican-agent
# Runs as root inside the container, but docker-compose drops every capability except NET_RAW.
ENTRYPOINT ["/usr/bin/lazy-pelican-agent"]
CMD ["run", "-config", "/etc/lazy-pelican-agent/config.yml"]
