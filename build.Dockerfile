FROM alpine:3.22

ARG TARGETPLATFORM

# Needed to reach HTTPS endpoints when is_secure = True.
RUN apk add --no-cache ca-certificates

COPY $TARGETPLATFORM/s3t /usr/bin/s3t

ENTRYPOINT ["/usr/bin/s3t"]
