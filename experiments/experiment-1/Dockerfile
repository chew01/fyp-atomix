FROM golang:1.24.6-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY *.go .

RUN CGO_ENABLED=0 GOOS=linux go build -o atomix-app ./main.go

CMD ["./atomix-app"]
