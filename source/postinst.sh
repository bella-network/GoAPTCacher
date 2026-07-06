#!/bin/sh
# postinst script
set -e

unit_exists() {
	unit="$1"

	[ -e "/lib/systemd/system/$unit" ] || [ -e "/usr/lib/systemd/system/$unit" ] || [ -e "/etc/systemd/system/$unit" ]
}

enable_unit() {
	unit="$1"

	if ! unit_exists "$unit"; then
		return 0
	fi

	if [ -z "${2:-}" ] || deb-systemd-helper --quiet was-enabled "$unit"; then
		deb-systemd-helper enable "$unit" >/dev/null || true
	else
		deb-systemd-helper update-state "$unit" >/dev/null || true
	fi
}

case "$1" in
	configure)
		if ! id "goaptcacher" >/dev/null 2>&1; then
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

for unit in goaptcacher.service goaptcacher-repoverify.timer; do
	if ! unit_exists "$unit"; then
		continue
	fi

	deb-systemd-helper unmask "$unit" >/dev/null || true
	enable_unit "$unit" "${2:-}"

	if deb-systemd-helper --quiet is-enabled "$unit" >/dev/null; then
		systemctl restart "$unit" >/dev/null || true
	fi
done

exit 0
