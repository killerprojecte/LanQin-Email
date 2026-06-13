FROM golang:1.22-bookworm AS build
WORKDIR /src/apps/api
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/lanqin-api ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/lanqin-api /usr/local/bin/lanqin-api
EXPOSE 8080
CMD ["lanqin-api"]
