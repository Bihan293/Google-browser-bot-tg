# --- build stage ---
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/bot ./cmd/bot

# --- runtime stage ---
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata && update-ca-certificates
WORKDIR /app
COPY --from=build /out/bot /app/bot
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/bot"]
