# Multi-stage: api, worker, bot in one image (разные command в compose)

FROM golang:1.25-alpine AS build

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/bot ./cmd/bot && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/seed ./cmd/seed && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/generate_seed ./cmd/generate_seed

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata curl

COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/worker /usr/local/bin/worker
COPY --from=build /out/bot /usr/local/bin/bot
COPY --from=build /out/seed /usr/local/bin/seed
COPY --from=build /out/generate_seed /usr/local/bin/generate_seed

# default — переопределяется в compose
CMD ["api"]
