FROM alpine:latest as alpine
RUN apk add -U --no-cache ca-certificates

FROM scratch
LABEL maintainer="dmitrys@xyzrd.com" \
org.label-schema.name="MX Proxy Service" \
org.label-schema.vendor="xyzrd.com" \
org.label-schema.vcs-url="https://github.com/Connector73/mxproxy" \
org.label-schema.schema-version="1.0"
COPY --from=alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY mxproxy /
ENV PORT="8080" LOG="" ADMIN="8049"
EXPOSE ${PORT}
EXPOSE ${ADMIN}
VOLUME ["/config", "/db"]
ENTRYPOINT ["/mxproxy"]
