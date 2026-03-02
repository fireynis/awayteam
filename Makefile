.PHONY: build run dev clean frontend test

build: frontend
	go build -o aid ./cmd/aid

run: build
	./aid serve

dev:
	go run ./cmd/aid serve

frontend:
	cd web && npm run build
	rm -rf internal/frontend/dist
	cp -r web/out internal/frontend/dist

clean:
	rm -f aid
	rm -rf internal/frontend/dist
	rm -rf web/out web/.next

test:
	go test ./...
