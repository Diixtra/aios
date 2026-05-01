# syntax=docker/dockerfile:1
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/auth-broker ./cmd

FROM node:24-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tini \
  && rm -rf /var/lib/apt/lists/* \
  && npm install -g @mariozechner/pi-coding-agent@0.70.6
COPY --from=build /out/auth-broker /usr/local/bin/auth-broker
ENV AUTH_BROKER_PI_BINARY=/usr/local/bin/pi
ENV AUTH_BROKER_PI_DIR=/pi-state
USER 65532:65532
ENTRYPOINT ["/usr/bin/tini","--"]
CMD ["/usr/local/bin/auth-broker"]
