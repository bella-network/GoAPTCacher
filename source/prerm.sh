#!/bin/sh
# postrm script
# see: dh_installdeb(1)
set -e

if [ "$1" = "remove" ] || [ "$1" = "deconfigure" ] ; then
	systemctl stop goaptcacher-repoverify.timer || true
	systemctl disable goaptcacher-repoverify.timer || true
	systemctl stop goaptcacher.service
	systemctl disable goaptcacher.service
fi

exit 0
