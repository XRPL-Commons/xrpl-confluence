FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/fuzz ./cmd/fuzz

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /out/fuzz /fuzz
EXPOSE 8081
ENTRYPOINT ["/fuzz"]
