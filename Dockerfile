# syntax=docker/dockerfile:1

FROM oven/bun:1 AS web
WORKDIR /web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/static ./web/static
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/server .

FROM gcr.io/distroless/static-debian12 AS runtime
WORKDIR /app
COPY --from=build /app/server ./server
COPY --from=build /src/web/templates ./web/templates
COPY --from=build /src/web/static ./web/static
EXPOSE 8080
ENV ADDR=:8080
ENV GIN_MODE=release
ENTRYPOINT ["./server"]
