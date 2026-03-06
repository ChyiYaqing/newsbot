APP       := newsbot
IMAGE     := chyiyaqing/$(APP)
FE_IMAGE  := chyiyaqing/$(APP)-frontend
TAG       := $(shell git describe --tags --abbrev=0 2>/dev/null || echo latest)
PLATFORMS := linux/amd64,linux/arm64

.PHONY: build run clean lint \
        docker docker-push docker-buildx docker-run \
        fe-install fe-dev fe-build fe-docker fe-docker-push fe-docker-buildx \
        up down logs

# ── Backend ────────────────────────────────────────────────────────────────

build:
	go build -trimpath -ldflags="-s -w" -o $(APP) .

run: build
	./$(APP) run

clean:
	rm -f $(APP)
	rm -rf frontend/dist

lint:
	go vet ./...

docker:
	docker build -t $(IMAGE):$(TAG) .

docker-push:
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE):$(TAG) --push .

docker-buildx:
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE):$(TAG) .

docker-run:
	docker run --rm -it \
		-p 8080:8080 \
		-v $(PWD)/newsbot.yaml:/app/newsbot.yaml:ro \
		-v $(PWD)/.env:/app/.env:ro \
		-v $(PWD)/data:/app/data \
		$(IMAGE):$(TAG)

# ── Frontend ───────────────────────────────────────────────────────────────

fe-install:
	cd frontend && npm install

fe-dev: fe-install
	cd frontend && npm run dev

fe-build: fe-install
	cd frontend && npm run build

fe-docker:
	docker build -t $(FE_IMAGE):$(TAG) ./frontend

fe-docker-push:
	docker buildx build --platform $(PLATFORMS) -t $(FE_IMAGE):$(TAG) --push ./frontend

fe-docker-buildx:
	docker buildx build --platform $(PLATFORMS) -t $(FE_IMAGE):$(TAG) ./frontend

# ── Compose ────────────────────────────────────────────────────────────────

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f
