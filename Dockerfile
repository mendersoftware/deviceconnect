FROM golang:1.16.0-alpine3.12 as builder
RUN apk add --no-cache \
    xz-dev \
    musl-dev \
    gcc
RUN mkdir -p /go/src/github.com/mendersoftware/deviceconnect
COPY . /go/src/github.com/mendersoftware/deviceconnect
RUN cd /go/src/github.com/mendersoftware/deviceconnect && env CGO_ENABLED=1 go build

FROM alpine:3.13.0
RUN apk add --no-cache ca-certificates xz
RUN mkdir -p /etc/deviceconnect
COPY ./config.yaml /etc/deviceconnect
COPY --from=builder /go/src/github.com/mendersoftware/deviceconnect/deviceconnect /usr/bin
ENTRYPOINT ["/usr/bin/deviceconnect", "--config", "/etc/deviceconnect/config.yaml"]

EXPOSE 8080
