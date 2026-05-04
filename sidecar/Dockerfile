FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/fuzz ./cmd/fuzz && \
    mkdir -p /out/output/corpus && \
    chown -R 65532:65532 /out/output

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /out/fuzz /fuzz
COPY --from=builder --chown=65532:65532 /out/output /output
EXPOSE 8081
ENTRYPOINT ["/fuzz"]
