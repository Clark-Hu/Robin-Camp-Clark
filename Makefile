.PHONY: docker-up docker-down test-e2e

docker-up:
	sudo docker compose up -d --build

docker-down:
	sudo docker compose down -v

wait-for-health:
	./scripts/wait-for-health.sh

test-e2e: wait-for-health
	./e2e-test.sh

migrate:
    sudo docker compose run --rm migrate