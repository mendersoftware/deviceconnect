FROM golang:1.18.1-alpine3.15 as builder
WORKDIR /go/src/github.com/mendersoftware/deviceconnect
RUN apk add --no-cache \
    xz-dev \
    musl-dev \
    gcc \
    ca-certificates
COPY ./ .
RUN CGO_ENABLED=0 go build

FROM scratch
EXPOSE 8080
WORKDIR /etc/deviceconnect
COPY ./config.yaml .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/src/github.com/mendersoftware/deviceconnect/deviceconnect /usr/bin/

ENTRYPOINT ["/usr/bin/deviceconnect", "--config", "/etc/deviceconnect/config.yaml"]
