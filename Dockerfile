FROM golang:1.24.3-bookworm AS build

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=0
ENV GOOS=linux
ARG VERSION
RUN go build -trimpath -ldflags="-s -w -X main.Version=${VERSION:-development}" -o /out/kish ./cmd/kish

FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION
LABEL org.opencontainers.image.title="Kish ${VERSION:-development}" \
      org.opencontainers.image.authors="Chen-Hao Ku <maple52046@gmail.com>" \
      org.opencontainers.image.description="Kish is a platform for collecting environment information and uploading test results." \
      org.opencontainers.image.source="https://github.com/AFDEAPAC/kish" \
      org.opencontainers.image.vendor="Chen-Hao Ku <maple52046@gmail.com>" \
      org.opencontainers.image.version="${VERSION:-development}"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/kish /usr/local/bin/kish
COPY docs/example-config.yaml /etc/kish/example-config.yaml

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/kish"]
