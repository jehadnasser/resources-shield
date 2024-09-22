FROM golang:1.22 AS builder

WORKDIR /app

COPY ./app/go.mod ./app/go.sum ./
RUN go mod download

COPY ./app/ .

RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -a -o resources-shield main.go

FROM alpine:3.14

RUN apk --no-cache add ca-certificates

COPY --from=builder /app/resources-shield /resources-shield

CMD ["/resources-shield"]
