# Build context is the goXRPL repo root (COPY . . pulls goXRPL source):
#   docker build -f goxrpl-race.Dockerfile -t goxrpl:race <path-to-goXRPL>
# Produces the race-detector goXRPL image used by scenarios/soak-escrow-race.yaml.
#
# -race-instrumented goXRPL image for soak/debug runs.
#
# The production Dockerfile builds a fully-static distroless binary, which is
# incompatible with the Go race detector (it needs CGO + dynamic linking and a
# glibc runtime). This image uses a glibc (debian) base and builds+runs in the
# same image so the CGO runtime deps (OpenSSL, libsecp256k1, the race runtime)
# are guaranteed present. It is intentionally large — it is a debug artifact,
# not the production image.
#
# Data races print "WARNING: DATA RACE" to stderr (the container log). With the
# default GORACE the node keeps running so a soak surfaces races without the
# mesh dying mid-consensus; grep the goxrpl logs for "DATA RACE".
FROM golang:1.24-bookworm

ARG LIBSECP256K1_VERSION=v0.5.0

RUN apt-get update && apt-get install -y --no-install-recommends \
        git gcc make pkg-config autoconf automake libtool libssl-dev ca-certificates \
 && git clone --depth 1 --branch ${LIBSECP256K1_VERSION} \
        https://github.com/bitcoin-core/secp256k1.git /tmp/secp256k1 \
 && cd /tmp/secp256k1 \
 && ./autogen.sh \
 && ./configure --enable-static --disable-shared \
        --disable-tests --disable-benchmark --disable-exhaustive-tests \
        --enable-module-recovery=no \
 && make -j"$(nproc)" install \
 && ldconfig \
 && rm -rf /tmp/secp256k1 /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# -race requires CGO and dynamic linking (no -extldflags=-static).
RUN CGO_ENABLED=1 go build -race -trimpath -o /usr/local/bin/goxrpl ./cmd/xrpld

# Report races to stderr/log and keep running (don't halt the node mid-round).
ENV GORACE="halt_on_error=0 history_size=2"

EXPOSE 5005 5555 6005 6006 51235

ENTRYPOINT ["goxrpl"]
CMD ["server", "--conf", "/etc/goxrpl/xrpld.toml"]
