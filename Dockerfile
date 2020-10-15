FROM golang:alpine as builder

WORKDIR /build

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN go build

FROM alpine

WORKDIR /vwidentity

COPY --from=builder /build/vwidentity /usr/local/bin/vwidentity

CMD [ "vwidentity" ]