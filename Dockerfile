FROM golang:1.10-alpine as builder

RUN apk --update add --no-cache --virtual .build-deps \
    gcc libc-dev linux-headers

ENV SRC=/go/src/github.com/infobloxopen/atlas-db

COPY . ${SRC}
WORKDIR ${SRC}

RUN go build -o bin/atlas-db-controller ./atlas-db-controller

FROM alpine:3.5

ENV SRC=/go/src/github.com/infobloxopen/atlas-db
COPY --from=builder ${SRC}/bin/atlas-db-controller /

# Allows to verify certificates
RUN apk update --no-cache && apk add --no-cache ca-certificates

# To enable debug uncomment the next line comment the line after.
#ENTRYPOINT ["/atlas-db-controller", "-logtostderr", "-v", "4"]
ENTRYPOINT ["/atlas-db-controller"]
