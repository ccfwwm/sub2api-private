FROM RELEASE_IMAGE AS trusted
FROM postgres:18.6-alpine AS pgclient
FROM alpine:3.23.4
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates ffmpeg tzdata su-exec libpq zstd-libs lz4-libs krb5-libs libldap libedit && addgroup -g 1000 sub2api && adduser -u 1000 -G sub2api -s /bin/sh -D sub2api && mkdir -p /app/data && chown 1000:1000 /app/data
COPY --from=pgclient /usr/local/bin/pg_dump /usr/local/bin/psql /usr/local/bin/
COPY --from=pgclient /usr/local/lib/libpq.so.5* /usr/local/lib/
COPY --from=trusted --chown=1000:1000 /app/sub2api /app/sub2api
COPY --from=trusted --chown=1000:1000 /app/resources /app/resources
COPY --from=trusted /app/docker-entrypoint.sh /app/docker-entrypoint.sh
WORKDIR /app
USER 1000:1000
ENV TZ=Asia/Shanghai
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 CMD wget -q -T 5 -O /dev/null http://localhost:8080/health || exit 1
ENTRYPOINT ["/app/sub2api"]
LABEL org.opencontainers.image.revision=RELEASE_COMMIT migration.security.patch=alpine323-20261117
