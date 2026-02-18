#!/bin/sh

set -e

# Flag to indicate if this is an upgrade
upgrade="false"

case "$1" in
	# DEB based systems
	configure)
		if [ -z "$2" ]; then
			upgrade="false"
		else
			upgrade="true"
		fi
		;;
	abort-deconfigure|abort-upgrade|abort-remove)
		;;
	# RPM based systems
	1)
		# Installation
		upgrade="false"
		;;
	2)
		# Upgrade
		upgrade="true"
		;;
	*)
		echo "[ERROR] post-install script called with unknown argument: '$1'"
		exit 1
		;;
esac

if command -v systemctl >/dev/null; then
	# Reload systemd configuration to make sure the new unit file gets activated
	systemctl daemon-reload || true
fi

if [ "$upgrade" = "true" ]; then
	# This is an upgrade, exit early.
	exit 0
fi

echo "################################################################################"
echo "Graylog Collector does NOT start automatically!"
echo ""
echo "Please run the following commands if you want to start the service automatically on system boot:"
echo ""
echo "    sudo systemctl enable graylog-collector.service"
echo ""
echo "    sudo systemctl start graylog-collector.service"
echo ""
echo "################################################################################"

exit 0
