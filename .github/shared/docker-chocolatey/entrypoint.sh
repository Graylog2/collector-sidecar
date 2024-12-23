#!/bin/sh -l

echo "Running chocolatey docker action with args $@"

cd /github/workspace && exec $@
