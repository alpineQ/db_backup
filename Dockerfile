FROM golang:1.15.1-alpine AS builder

WORKDIR /go/src/github.com/alpineQ/db_backup/

RUN apk add --no-cache git
COPY . .
RUN go get -d -v

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o main 

FROM scratch
COPY --from=builder /go/src/github.com/alpineQ/db_backup/ /

EXPOSE 8080
ENTRYPOINT [ "/main" ] 
