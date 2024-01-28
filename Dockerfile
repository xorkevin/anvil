FROM cgr.dev/chainguard/go:latest as builder
WORKDIR /usr/local/src/go/anvil
COPY --link go.mod /usr/local/src/go/anvil/go.mod
COPY --link go.sum /usr/local/src/go/anvil/go.sum
RUN go mod download -json && go mod verify
COPY --link . /usr/local/src/go/anvil
RUN GOPROXY=off CGO_ENABLED=0 go build -v -trimpath -ldflags "-w -s" -o /usr/local/bin/anvil .

FROM cgr.dev/chainguard/bash:latest
MAINTAINER xorkevin <kevin@xorkevin.com>
COPY --link --from=builder /usr/local/bin/anvil /usr/local/bin/anvil
WORKDIR /home/anvil
ENTRYPOINT ["/usr/local/bin/anvil"]
CMD ["workflow", "--config", "/home/anvil/config/.anvil.json", "--input", "/home/anvil/workflows/main.star"]
