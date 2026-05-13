#!/bin/sh

set -e

user="graylog-collector"
group="graylog-collector"
datadir="/var/lib/graylog-collector"
#logdir="/var/log/graylog-collector"

remove_data="false"

case "$1" in
	# DEB based systems
	remove)
		remove_data="false"
		;;

	purge)
		remove_data="true"
		;;

	upgrade|failed-upgrade|abort-install|abort-upgrade|disappear)
		# Nothing to do here
		;;
	# RPM based systems
	0)
		# Removal
		# We don't remove any data on package removal to ensure that
		# users don't lose any config data.
		remove_data="false"
		;;
	1)
		# Upgrade
		remove_data="false"
		;;
	*)
		echo "[ERROR] post-uninstall script called with unknown argument: '$1'"
		exit 1
		;;
esac

if [ "$remove_data" = "true" ]; then
	rm -rf "$datadir" #"$logdir"

	if id "$user" > /dev/null 2>&1 ; then
		userdel "$user" || true
	fi

	if getent group "$group" > /dev/null 2>&1 ; then
		groupdel "$group" || true
	fi
fi

exit 0
