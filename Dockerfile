FROM golang:1.15.1-alpine AS builder

ENV DOCKER_API_VERSION=1.40
WORKDIR /go/src/app

RUN apk add --no-cache git
COPY . .
RUN go get -d -v

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o main 

FROM alpine:latest
WORKDIR /app
COPY --from=builder /go/src/app/ /app/

EXPOSE 8080
ENTRYPOINT /app/main
