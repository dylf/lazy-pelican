FROM golang:1.24-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /lazy-agent ./cmd/lazy-agent

FROM gcr.io/distroless/static:latest
COPY --from=build /lazy-agent /usr/bin/lazy-agent
# Runs as root inside the container, but docker-compose drops every capability except NET_RAW.
ENTRYPOINT ["/usr/bin/lazy-agent"]
CMD ["run", "-config", "/etc/lazy-agent/config.yml"]
