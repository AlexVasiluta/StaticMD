FROM golang:alpine AS builder
RUN apk add git
WORKDIR /app
COPY . . 
RUN go get -v .
RUN go build -o output .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/output .
CMD ["./output"]
