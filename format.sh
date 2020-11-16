#!/bin/sh

root=$(dirname $0)
gofmt=${GOFMT:-gofmt}

add_license()
{
	echo "=> Adding license header to $1"
	ed -s "$1" <<-EOF
0a
// Copyright (C) 2020 Graylog, Inc.

// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.

.
w
	EOF
}

for file in $(find "$root" -name '*.go' | fgrep -v vendor/); do
	$gofmt -w -l "$file" && \
		grep -q 'under the terms of the GNU General Public License' $file || \
		add_license "$file"
done

exit 0
