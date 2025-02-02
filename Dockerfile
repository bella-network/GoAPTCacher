FROM debian:stable-slim AS base

RUN apt-get update && apt-get install -y \
	ca-certificates tzdata

RUN adduser \
	--disabled-password \
	--gecos "" \
	--home "/nonexistent" \
	--shell "/sbin/nologin" \
	--no-create-home \
	--uid 65532 \
	goaptcacher

FROM scratch

COPY --from=base /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=base /etc/passwd /etc/passwd
COPY --from=base /etc/group /etc/group

COPY goaptcacher /goaptcacher
COPY config.yaml-example /config/config.yaml

USER goaptcacher:goaptcacher

EXPOSE 8090
EXPOSE 8091

VOLUME [ "/cache", "/config" ]
WORKDIR /config

CMD ["/goaptcacher"]
