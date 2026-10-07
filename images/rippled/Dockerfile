# Runtime image for a participant-supplied, precompiled rippled binary.
# Build context must contain the executable as ./rippled; the repository helper
# stages an external binary into that context. The binary must target this
# image's Linux architecture and Ubuntu 22.04 glibc/C++ ABI.

FROM ubuntu:22.04

ARG RIPPLED_BINARY=rippled

# Common Ubuntu/Debian rippled runtime libraries. Add other dependencies to a
# derivative image when ldd or the preflight reports that the binary needs them.
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        libatomic1 \
        libboost-chrono1.74.0 \
        libboost-filesystem1.74.0 \
        libboost-program-options1.74.0 \
        libboost-regex1.74.0 \
        libboost-serialization1.74.0 \
        libboost-system1.74.0 \
        libboost-thread1.74.0 \
        libcurl4 \
        libicu70 \
        libjemalloc2 \
        libjsoncpp25 \
        libprotobuf23 \
        libssl3 \
        libunwind8 \
        zlib1g \
    && rm -rf /var/lib/apt/lists/*

COPY ${RIPPLED_BINARY} /usr/local/bin/rippled

RUN chmod 0755 /usr/local/bin/rippled \
    && test -x /usr/local/bin/rippled \
    && (ldd /usr/local/bin/rippled >/tmp/rippled-ldd 2>&1 || true) \
    && ! grep -q 'not found' /tmp/rippled-ldd
RUN /usr/local/bin/rippled --version

# Kurtosis mounts generated config under /etc/rippled and mounts the persistent
# database directory separately. Keep root so an empty engine-managed directory
# is writable across Docker and other Kurtosis backends.
RUN mkdir -p /var/lib/rippled/db

WORKDIR /var/lib/rippled

# 51235 = peer protocol; 5005 = HTTP JSON-RPC; 6006 = admin WebSocket.
EXPOSE 51235 5005 6006

ENTRYPOINT ["/usr/local/bin/rippled"]
