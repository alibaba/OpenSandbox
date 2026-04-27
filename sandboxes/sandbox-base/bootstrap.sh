#!/bin/sh
set -e
/opt/opensandbox/execd >/tmp/execd.log 2>&1 &
exec "$@"
