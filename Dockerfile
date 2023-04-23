FROM cgr.dev/chainguard/go:1.20.3 as builder
WORKDIR /usr/local/src/go/anvil
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . ./
RUN CGO_ENABLED=0 go build -v -trimpath -ldflags "-w -s" -o /usr/local/bin/anvil .

FROM cgr.dev/chainguard/busybox:latest
MAINTAINER xorkevin <kevin@xorkevin.com>
WORKDIR /home/anvil
COPY --from=builder /usr/local/bin/anvil /usr/local/bin/
VOLUME /home/anvil/config
VOLUME /home/anvil/workflows
VOLUME /home/anvil/output
ENTRYPOINT ["/usr/local/bin/anvil"]
CMD ["workflow", "--config", "/home/anvil/config/.anvil.json", "--input", "/home/anvil/workflows/main.star"]
