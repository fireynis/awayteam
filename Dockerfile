# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS backend
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /build/web/out ./internal/frontend/dist
RUN go build -o awayteam ./cmd/awayteam

# Stage 3: Runtime
FROM alpine:3.21
LABEL org.opencontainers.image.source=https://github.com/fireynis/awayteam
RUN apk add --no-cache ca-certificates
COPY --from=backend /build/awayteam /usr/local/bin/awayteam
VOLUME /data
ENV AWAYTEAM_DB_PATH=/data/awayteam.db
EXPOSE 8080
ENTRYPOINT ["awayteam"]
CMD ["serve"]
