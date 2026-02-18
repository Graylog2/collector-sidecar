#!/bin/sh

set -e

stopping="false"

case "$1" in
	# DEB based systems
	remove)
		stopping="true"
		;;
	upgrade)
		stopping="false"
		;;
	deconfigure|failed-upgrade)
		;;
	# RPM based systems
	0)
		# Removal
		stopping="true"
		;;
	1)
		# Upgrade
		stopping="false"
		;;
	*)
		echo "[ERROR] pre-uninstall script called with unknown argument: '$1'"
		exit 1
		;;
esac

if [ "$stopping" = "false" ]; then
	# Nothing to stop, exit early.
	exit 0
fi

systemctl --no-reload stop graylog-collector.service >/dev/null 2>&1 || true
systemctl disable graylog-collector >/dev/null 2>&1 || true

exit 0
