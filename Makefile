AIR_BIN := go run github.com/air-verse/air@latest

run-dev:
	$(AIR_BIN)
.PHONY: run-dev

build-docker:
	docker build -t trojan295/airplay .
.PHONY: build-docker

push-docker: build-docker
	docker push trojan295/airplay
.PHONY: push-docker
