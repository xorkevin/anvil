FROM cgr.dev/chainguard/go:latest as builder
WORKDIR /usr/local/src/go/anvil
COPY --link go.mod /usr/local/src/go/anvil/go.mod
COPY --link go.sum /usr/local/src/go/anvil/go.sum
RUN \
  --mount=type=cache,id=gomodcache,sharing=locked,target=/root/go/pkg/mod \
  go mod download -json && go mod verify
COPY --link . /usr/local/src/go/anvil
RUN \
  --mount=type=cache,id=gomodcache,sharing=locked,target=/root/go/pkg/mod \
  --mount=type=cache,id=gobuildcache,sharing=locked,target=/root/.cache/go-build \
  GOPROXY=off go build -v -trimpath -ldflags "-w -s" -o /usr/local/bin/anvil .

FROM cgr.dev/chainguard/bash:latest
LABEL org.opencontainers.image.authors="Kevin Wang <kevin@xorkevin.com>"
COPY --link --from=builder /usr/local/bin/anvil /usr/local/bin/anvil
WORKDIR /home/anvil
ENTRYPOINT ["/usr/local/bin/anvil"]
CMD ["workflow", "--config", "/home/anvil/config/.anvil.json", "--input", "/home/anvil/workflows/main.star"]
