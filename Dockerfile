FROM golang:1.16 as gobuilder

WORKDIR /root

ENV GOOS=linux\
    GOARCH=amd64

COPY go.mod .
COPY go.sum .

RUN go mod edit -replace github.com/fluent/fluent-bit-go=github.com/fluent/fluent-bit-go@master
RUN go mod download

COPY . .

RUN make build-plugin

FROM fluent/fluent-bit:1.8.11

COPY --from=gobuilder /root/out_dogstatsd_metrics.so /fluent-bit/bin/
COPY --from=gobuilder /root/fluent-bit.conf /fluent-bit/etc/
COPY --from=gobuilder /root/plugins.conf /fluent-bit/etc/

EXPOSE 2020

CMD ["/fluent-bit/bin/fluent-bit", "--config", "/fluent-bit/etc/fluent-bit.conf"]