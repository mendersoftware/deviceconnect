FROM --platform=$BUILDPLATFORM golang:1.23.9-alpine3.20 AS builder
ARG TARGETARCH
WORKDIR /go/src/github.com/mendersoftware/deviceconnect
RUN apk add --no-cache \
    ca-certificates
COPY ./ .
RUN CGO_ENABLED=0 GOARCH=$TARGETARCH go build

FROM scratch
EXPOSE 8080
USER 65534:65534
WORKDIR /etc/deviceconnect
COPY --from=builder --chown=65534:65534 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --chown=65534:65534 ./config.yaml .
COPY --from=builder --chown=65534:65534 /go/src/github.com/mendersoftware/deviceconnect/deviceconnect /usr/bin/

ENTRYPOINT ["/usr/bin/deviceconnect", "--config", "/etc/deviceconnect/config.yaml"]
