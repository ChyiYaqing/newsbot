APP      := newsbot
IMAGE    := chyiyaqing/$(APP)
TAG      := latest
PLATFORMS := linux/amd64,linux/arm64

.PHONY: build run clean docker docker-run docker-push up down lint

build:
	go build -trimpath -ldflags="-s -w" -o $(APP) .

run: build
	./$(APP) run

clean:
	rm -f $(APP)

lint:
	go vet ./...

docker:
	docker build -t $(IMAGE):$(TAG) .

docker-buildx:
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE):$(TAG) --push .

docker-push: docker
	docker push $(IMAGE):$(TAG)

docker-run:
	docker run --rm -it \
		-p 8080:8080 \
		-v $(PWD)/newsbot.yaml:/app/newsbot.yaml:ro \
		-v $(PWD)/.env:/app/.env:ro \
		-v $(PWD)/data:/app/data \
		$(IMAGE):$(TAG)

up:
	docker compose up -d --build

down:
	docker compose down
