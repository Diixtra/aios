FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ticktick-sync ./cmd/

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /ticktick-sync /ticktick-sync
USER 65532:65532
ENTRYPOINT ["/ticktick-sync"]
