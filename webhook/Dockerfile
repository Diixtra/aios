FROM golang:1.23-alpine AS builder

WORKDIR /workspace

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o webhook ./cmd/main.go

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /workspace/webhook /webhook

USER 65532:65532

ENTRYPOINT ["/webhook"]
