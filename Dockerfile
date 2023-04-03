# Build the manager binary
FROM golang:1.19.2 as builder

WORKDIR /code

COPY . .

RUN go mod download


RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o app ./

FROM reg.navercorp.com/base/alpine:3.14
WORKDIR /
COPY --from=builder /code/app .
USER 65532:65532

ENTRYPOINT ["/app"]
