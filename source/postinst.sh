#!/bin/sh
# postinst script
set -e

enable_unit() {
	unit="$1"

	if [ -z "${2:-}" ] || deb-systemd-helper --quiet was-enabled "$unit"; then
		deb-systemd-helper enable "$unit" >/dev/null || true
	else
		deb-systemd-helper update-state "$unit" >/dev/null || true
	fi
}

case "$1" in
	configure)
		if ! id "goaptcacher" &>/dev/null; then
			useradd --system --no-create-home --shell /bin/false goaptcacher
			echo "User goaptcacher created."
		else
			echo "User goaptcacher already exists."
		fi

		# Create the cache directory and set ownership
		mkdir -p /var/cache/goaptcacher
		chown goaptcacher:goaptcacher /var/cache/goaptcacher
		echo "Cache directory /var/cache/goaptcacher created and ownership set to goaptcacher."
		;;
esac

if [ "$1" = "triggered" ]; then
	invoke-rc.d goaptcacher.service restart
fi

systemctl daemon-reload >/dev/null || true

deb-systemd-helper unmask goaptcacher.service >/dev/null || true
deb-systemd-helper unmask goaptcacher-repoverify.timer >/dev/null || true

enable_unit goaptcacher.service "${2:-}"
enable_unit goaptcacher-repoverify.timer "${2:-}"

if deb-systemd-helper --quiet is-enabled goaptcacher.service >/dev/null; then
	systemctl restart goaptcacher.service >/dev/null || true
fi

if deb-systemd-helper --quiet is-enabled goaptcacher-repoverify.timer >/dev/null; then
	systemctl restart goaptcacher-repoverify.timer >/dev/null || true
fi

exit 0
