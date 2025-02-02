#!/bin/sh
# postinst script

if [ "$1" = "triggered" ]; then
	invoke-rc.d goaptcacher.service restart
fi

deb-systemd-helper unmask goaptcacher.service >/dev/null || true

if deb-systemd-helper --quiet was-enabled goaptcacher.service; then
	# Enables the unit on first installation, creates new
	# symlinks on upgrades if the unit file has changed.
	deb-systemd-helper enable goaptcacher.service >/dev/null || true
else
	# Update the statefile to add new symlinks (if any), which need to be
	# cleaned up on purge. Also remove old symlinks.
	deb-systemd-helper update-state goaptcacher.service >/dev/null || true
fi

if deb-systemd-helper --quiet is-enabled goaptcacher.service >/dev/null; then
	systemctl restart goaptcacher.service >/dev/null || true
fi

exit 0
