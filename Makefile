ENV_FILE := deploy/.env
ENV_EXAMPLE := deploy/.env.example
COMPOSE := docker compose --env-file $(ENV_FILE) -f deploy/compose.yaml
SECRET_FILE := secrets/postgres_password

.PHONY: infra-env infra-init infra-config infra-up infra-status infra-check infra-logs infra-down

infra-env:
	@if [ ! -f "$(ENV_FILE)" ]; then \
		cp "$(ENV_EXAMPLE)" "$(ENV_FILE)"; \
		printf 'Created %s from %s\n' "$(ENV_FILE)" "$(ENV_EXAMPLE)"; \
	fi

infra-init: infra-env
	@mkdir -p secrets
	@if [ ! -s "$(SECRET_FILE)" ]; then \
		umask 077; \
		openssl rand -out "$(SECRET_FILE)" -hex 32; \
		printf 'Created %s\n' "$(SECRET_FILE)"; \
	fi
	@chmod 600 "$(SECRET_FILE)"

infra-config: infra-init
	@$(COMPOSE) config --quiet

infra-up: infra-config
	@$(COMPOSE) up -d --build --remove-orphans
	@$(COMPOSE) ps

infra-status:
	@$(COMPOSE) ps

infra-check:
	@curl --fail --silent --show-error http://127.0.0.1:8080/healthz
	@printf '\n'
	@curl --fail --silent --show-error http://127.0.0.1:8080/readyz
	@printf '\n'
	@curl --fail --silent --show-error http://127.0.0.1:8080/v1/status
	@printf '\n'

infra-logs:
	@$(COMPOSE) logs --tail=200 -f trader postgres migrate

infra-down:
	@$(COMPOSE) down --remove-orphans
