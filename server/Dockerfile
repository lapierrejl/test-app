# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS build

WORKDIR /app
COPY . .
RUN go build -o stress-app main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=build /app/stress-app /stress-app
EXPOSE 8080
CMD ["/stress-app"]