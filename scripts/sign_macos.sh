#!/usr/bin/env bash

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

codesign -s $AC_APPLICATION_IDENTITY -f -v --timestamp --options runtime $1 

config="$(mktemp -d)/gon.json" 
jq -r --null-input --arg path "$(pwd)/$1" '{
    "notarize": [
        {
            "path": $path,
            "bundle_id": "com.coder.cli"
        }
    ]
}' > $config
gon $config
