FROM golang:alpine AS builder
WORKDIR /app
COPY go.mod go.sum main.go js_runner.go ./
RUN go get -v .
RUN go build -o output .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/output .
CMD ["./output"]
