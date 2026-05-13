#!/bin/sh

set -e

user="graylog-collector"
group="graylog-collector"
datadir="/var/lib/graylog-collector"
#logdir="/var/log/graylog-collector"

add_group() {
	if ! getent group "$user" >/dev/null; then
		groupadd -r "$user"
	fi
}

add_user() {
	if ! getent passwd "$user">/dev/null; then
		useradd -r -M -g "$group" -d "$datadir" \
			-s /sbin/nologin -c "Graylog Collector" "$user"
	fi
}

case "$1" in
	# DEB based systems
	install|upgrade)
		add_group
		add_user
		;;
	abort-deconfigure|abort-upgrade|abort-remove)
		# Ignore
		;;
	# RPM based systems
	1|2)
		add_group
		add_user
		;;
	*)
		echo "[ERROR] pre-install script called with unknown argument: '$1'"
		exit 1
		;;
esac

# Create directories
install -d -o "$user" -g "$group" -m 0755 "$datadir"
#install -d -o "$user" -g "$group" -m 0755 "$logdir"

exit 0
