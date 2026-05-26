# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/warden ./cmd/server

FROM node:22-alpine AS frontend-builder
WORKDIR /frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ .
RUN npm run build

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/warden /warden
COPY --from=frontend-builder /frontend/dist /frontend/dist
COPY internal/db/migrations /internal/db/migrations
EXPOSE 8080
ENTRYPOINT ["/warden"]
