#!/bin/sh
# postrm script

set -e
# _manupdate() {
#       mandb -f /usr/share/man/man1/them-sslutils.1.gz
#}

# summary of how this script can be called:
#        * <postrm> `remove'
#        * <postrm> `purge'
#        * <old-postrm> `upgrade' <new-version>
#        * <new-postrm> `failed-upgrade' <old-version>
#        * <new-postrm> `abort-install'
#        * <new-postrm> `abort-install' <old-version>
#        * <new-postrm> `abort-upgrade' <old-version>
#        * <disappearer's-postrm> `disappear' <overwriter>
#          <overwriter-version>
# for details, see http://www.debian.org/doc/debian-policy/ or
# the debian-policy package

# Reload systemd
systemctl daemon-reload || :

if [ "$1" = "remove" ] ; then
  rm -f /lib/systemd/system/goaptcacher.service
fi

if [ "$1" = "purge" ] ; then
  rm -rf /etc/goaptcacher
fi

exit 0
