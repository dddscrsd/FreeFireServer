# Go match-server — CGO off => a static binary in a distroless image. One image
# per fleet instance; the matchmaker allocates matches to instances (they are NOT
# load-balanced). Build context is the repo root.
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY match-server/go.mod match-server/go.sum ./
RUN go mod download
COPY match-server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/match-server .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/match-server /match-server
EXPOSE 10100/udp
ENTRYPOINT ["/match-server"]
CMD ["-addr", ":10100"]
