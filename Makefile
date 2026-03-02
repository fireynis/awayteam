.PHONY: build run dev clean frontend test

build: frontend
	go build -o awayteam ./cmd/awayteam

run: build
	./awayteam serve

dev:
	go run ./cmd/awayteam serve

frontend:
	cd web && npm run build
	rm -rf internal/frontend/dist
	cp -r web/out internal/frontend/dist

clean:
	rm -f awayteam
	rm -rf internal/frontend/dist
	rm -rf web/out web/.next

test:
	go test ./...
