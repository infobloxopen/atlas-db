FROM alpine:3.5

COPY bin/atlas-db-controller /

# Allows to verify certificates
RUN apk update --no-cache && apk add --no-cache ca-certificates

ENTRYPOINT ["/atlas-db-controller"]
