FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /trafficgen ./cmd/trafficgen

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /trafficgen /trafficgen
EXPOSE 8081
ENTRYPOINT ["/trafficgen"]
