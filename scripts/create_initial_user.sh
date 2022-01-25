#!/bin/bash

set -euo pipefail

EMAIL="${EMAIL:-admin@coder.com}"
USERNAME="${USERNAME:-admin}"
ORGANIZATION="${ORGANIZATION:-default}"
PASSWORD="${PASSWORD:-p@ssword1}"

curl -X POST \
-d "{\"email\": \"$EMAIL\", \"username\": \"$USERNAME\", \"organization\": \"$ORGANIZATION\", \"password\": \"$PASSWORD\"}" \
-H 'Content-Type:application/json' \
http://localhost:3000/api/v2/users