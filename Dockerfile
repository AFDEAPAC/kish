FROM golang:1.24.3-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/kish ./cmd/kish

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/kish /kish

USER nonroot:nonroot
ENTRYPOINT ["/kish"]
