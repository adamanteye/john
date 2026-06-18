FROM golang AS multijohn-builder
WORKDIR /go/src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o multijohn

FROM ghcr.io/adamanteye/jtr:latest
WORKDIR /go/src
COPY --from=multijohn-builder /go/src .
CMD ["./multijohn"]
