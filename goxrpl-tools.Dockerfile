# goxrpl-tools — goXRPL binary + chaos-event tools (iproute2, iptables).
# Built by scripts/build-goxrpl-tools.sh; consumed by the chaos suite to
# enable LatencyEvent / PartitionEvent against goXRPL containers (the vanilla
# distroless goxrpl image lacks tc/iptables).

FROM goxrpl:latest AS bin

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends iproute2 iptables ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=bin /usr/local/bin/goxrpl /usr/local/bin/goxrpl

# 5005  = RPC admin
# 5555  = RPC public
# 6005  = WebSocket public
# 6006  = WebSocket admin
# 51235 = peer protocol
EXPOSE 5005 5555 6005 6006 51235

ENTRYPOINT ["goxrpl"]
CMD ["server", "--conf", "/etc/goxrpl/xrpld.toml"]
