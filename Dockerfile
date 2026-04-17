# Stage 1: build Svelte frontend
FROM node:20-alpine AS web-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/src ./src
COPY web/index.html web/vite.config.js web/tailwind.config.js web/postcss.config.js ./
RUN npm run build

# Stage 2: build Go binary
FROM golang:1.25-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /app/web/dist ./web/dist
RUN go build -o bin/server ./cmd/server

# Stage 3: minimal runtime image
FROM alpine:3.19
# esptool installed via pip (not apk) to get a current release.
# --break-system-packages is required on Alpine's managed Python.
RUN apk add --no-cache ca-certificates python3 py3-pip && \
    pip3 install "esptool==4.8.1" "esp-idf-nvs-partition-gen==0.1.5" --break-system-packages
WORKDIR /app
COPY --from=go-builder /app/bin/server .
COPY data/ ./data/
EXPOSE 48060 48061
ENV DATA_DIR=/data
CMD ["./server"]
