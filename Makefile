ENCLAVE       ?= xrpl-soak
GOXRPL_COUNT  ?= 2
RIPPLED_COUNT ?= 3
TX_RATE       ?= 5
ACCOUNTS      ?= 50
ROTATE_EVERY  ?= 1000
MUTATION_RATE ?= 0.05
CORPUS        ?= $(PWD)/.soak-corpus

# Optional sidecars / integrations.
OBSERVABILITY     ?= 0
ALERT_WEBHOOK_URL ?=

OBSERVABILITY_BOOL := $(if $(filter 1 true yes,$(OBSERVABILITY)),true,false)

.PHONY: soak soak-down soak-tail soak-status soak-pull soak-pull-loop chaos chaos-down chaos-tail chaos-pull chaos-pull-loop docker-proxy docker-proxy-down

soak:
	@bash scripts/build-sidecar.sh
	kurtosis enclave rm -f $(ENCLAVE) >/dev/null 2>&1 || true
	kurtosis run --enclave $(ENCLAVE) . '{"test_suite":"soak","goxrpl_count":$(GOXRPL_COUNT),"rippled_count":$(RIPPLED_COUNT),"soak_args":{"tx_rate":$(TX_RATE),"accounts":$(ACCOUNTS),"rotate_every":$(ROTATE_EVERY),"mutation_rate":$(MUTATION_RATE),"enable_observability":$(OBSERVABILITY_BOOL),"alert_webhook_url":"$(ALERT_WEBHOOK_URL)"}}'
	@DASH_IP=$$(kurtosis service inspect $(ENCLAVE) dashboard 2>/dev/null | awk '/IP Address/ {print $$3; exit}'); \
		if [ -n "$$DASH_IP" ]; then echo "Dashboard: http://$$DASH_IP:8080"; fi
	@echo "Tail logs: make soak-tail"
	@echo "Pull corpus: make soak-pull"

soak-down:
	kurtosis enclave rm -f $(ENCLAVE)

soak-tail:
	kurtosis service logs -f $(ENCLAVE) fuzz-soak

soak-status:
	kurtosis enclave inspect $(ENCLAVE)

soak-pull:
	@mkdir -p $(CORPUS)
	@echo "Extracting /output/corpus from fuzz-soak to $(CORPUS) ..."
	@UUID=$$(kurtosis service inspect $(ENCLAVE) fuzz-soak 2>/dev/null | awk '/^UUID:/ {print $$2; exit}'); \
		if [ -z "$$UUID" ]; then echo "fuzz-soak service not found"; exit 1; fi; \
		CONTAINER=$$(docker ps --format '{{.Names}}' | grep "^fuzz-soak--$$UUID" | head -1); \
		if [ -z "$$CONTAINER" ]; then echo "container for service uuid $$UUID not found"; exit 1; fi; \
		docker cp "$$CONTAINER:/output/corpus" $(CORPUS)/ 2>/dev/null && echo "Done." \
		|| echo "Extract failed (corpus exists? try waiting longer)"

CHAOS_ENCLAVE  ?= xrpl-chaos
CHAOS_SCHEDULE ?= $(PWD)/.chaos-schedule.json
CHAOS_CORPUS   ?= $(PWD)/.chaos-corpus

.PHONY: chaos chaos-down chaos-tail chaos-pull

chaos:
	@bash scripts/build-sidecar.sh
	@if [ ! -f $(CHAOS_SCHEDULE) ]; then echo "missing $(CHAOS_SCHEDULE) — copy .chaos-schedule.example.json or see docs/plans/2026-05-05-chaos-runner-m4.md"; exit 1; fi
	@SCHEDULE=$$(cat $(CHAOS_SCHEDULE) | tr -d '\n' | sed 's/"/\\"/g'); \
		kurtosis enclave rm -f $(CHAOS_ENCLAVE) >/dev/null 2>&1 || true; \
		kurtosis run --enclave $(CHAOS_ENCLAVE) . "{\"test_suite\":\"chaos\",\"goxrpl_count\":$(GOXRPL_COUNT),\"rippled_count\":$(RIPPLED_COUNT),\"chaos_args\":{\"schedule\":\"$$SCHEDULE\",\"tx_rate\":$(TX_RATE),\"accounts\":$(ACCOUNTS),\"rotate_every\":$(ROTATE_EVERY),\"mutation_rate\":$(MUTATION_RATE),\"enable_observability\":$(OBSERVABILITY_BOOL),\"alert_webhook_url\":\"$(ALERT_WEBHOOK_URL)\"}}"
	@echo "Tail logs: make chaos-tail"
	@echo "Pull corpus: make chaos-pull"

chaos-down:
	kurtosis enclave rm -f $(CHAOS_ENCLAVE)

chaos-tail:
	kurtosis service logs -f $(CHAOS_ENCLAVE) fuzz-chaos

chaos-pull:
	@mkdir -p $(CHAOS_CORPUS)
	@echo "Extracting /output/corpus from fuzz-chaos to $(CHAOS_CORPUS) ..."
	@UUID=$$(kurtosis service inspect $(CHAOS_ENCLAVE) fuzz-chaos 2>/dev/null | awk '/^UUID:/ {print $$2; exit}'); \
		if [ -z "$$UUID" ]; then echo "fuzz-chaos service not found"; exit 1; fi; \
		CONTAINER=$$(docker ps --format '{{.Names}}' | grep "^fuzz-chaos--$$UUID" | head -1); \
		if [ -z "$$CONTAINER" ]; then echo "container for service uuid $$UUID not found"; exit 1; fi; \
		docker cp "$$CONTAINER:/output/corpus" $(CHAOS_CORPUS)/ 2>/dev/null && echo "Done." \
		|| echo "Extract failed (corpus exists? try waiting longer)"

PULL_INTERVAL ?= 300

soak-pull-loop:
	bash scripts/corpus-pull-loop.sh $(ENCLAVE) fuzz-soak $(PULL_INTERVAL) $(CORPUS)

chaos-pull-loop:
	bash scripts/corpus-pull-loop.sh $(CHAOS_ENCLAVE) fuzz-chaos $(PULL_INTERVAL) $(CHAOS_CORPUS)

# tecnativa/docker-socket-proxy on the HOST. The Kurtosis enclave can't mount
# /var/run/docker.sock directly (1.x Starlark has no host-bind primitive), so
# the proxy runs as a host-level docker container exposing TCP 2375. The fuzz
# sidecar dials it via host.docker.internal (Docker Desktop on macOS provides
# this DNS by default; Linux uses --add-host=host.docker.internal:host-gateway
# baked into Docker 20.10+ via /etc/hosts).
#
# Whitelisted endpoints:
#   CONTAINERS=1  list/inspect/logs/kill/start/stop  (crash poller + restart event)
#   EXEC=1        ContainerExec                       (chaos latency / partition)
#   PING=1        liveness                            (NewDockerRuntime startup)
#   POST=1        all of the above are POST methods
DOCKER_PROXY_NAME ?= xrpl-confluence-docker-proxy

docker-proxy:
	@if docker ps --format '{{.Names}}' | grep -q "^$(DOCKER_PROXY_NAME)$$"; then \
		echo "$(DOCKER_PROXY_NAME) already running"; \
	else \
		docker run -d --rm --name $(DOCKER_PROXY_NAME) \
			-p 2375:2375 \
			-v /var/run/docker.sock:/var/run/docker.sock \
			-e CONTAINERS=1 -e EXEC=1 -e PING=1 -e POST=1 \
			tecnativa/docker-socket-proxy:latest >/dev/null && \
		echo "started $(DOCKER_PROXY_NAME) on tcp://host.docker.internal:2375"; \
	fi

docker-proxy-down:
	@docker rm -f $(DOCKER_PROXY_NAME) >/dev/null 2>&1 || true
	@echo "stopped $(DOCKER_PROXY_NAME)"
