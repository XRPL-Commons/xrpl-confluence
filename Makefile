ENCLAVE       ?= xrpl-soak
GOXRPL_COUNT  ?= 2
RIPPLED_COUNT ?= 3
TX_RATE       ?= 5
ACCOUNTS      ?= 50
ROTATE_EVERY  ?= 1000
MUTATION_RATE ?= 0.05
CORPUS        ?= $(PWD)/.soak-corpus

.PHONY: soak soak-down soak-tail soak-status soak-pull

soak:
	@bash scripts/build-sidecar.sh
	kurtosis enclave rm -f $(ENCLAVE) >/dev/null 2>&1 || true
	kurtosis run --enclave $(ENCLAVE) . '{"test_suite":"soak","goxrpl_count":$(GOXRPL_COUNT),"rippled_count":$(RIPPLED_COUNT),"soak_args":{"tx_rate":$(TX_RATE),"accounts":$(ACCOUNTS),"rotate_every":$(ROTATE_EVERY),"mutation_rate":$(MUTATION_RATE)}}'
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
