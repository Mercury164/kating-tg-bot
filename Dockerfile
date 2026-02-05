FROM golang:1.20-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/bot ./cmd/bot

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/bot /app/bot
COPY .env.example /app/.env.example
EXPOSE 8080
ENTRYPOINT ["/app/bot"]
