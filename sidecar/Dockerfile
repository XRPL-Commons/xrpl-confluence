FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/fuzz ./cmd/fuzz && \
    CGO_ENABLED=0 go build -o /out/confluence-control ./cmd/confluence-control && \
    mkdir -p /out/output/corpus && \
    mkdir -p /out/var/confluence/findings /out/var/confluence/logs /out/var/confluence/reproducers && \
    chown -R 65532:65532 /out/output /out/var

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /out/fuzz /fuzz
COPY --from=builder /out/confluence-control /confluence-control
COPY --from=builder --chown=65532:65532 /out/output /output
COPY --from=builder --chown=65532:65532 /out/var /var
EXPOSE 8081
EXPOSE 8090
ENTRYPOINT ["/fuzz"]
