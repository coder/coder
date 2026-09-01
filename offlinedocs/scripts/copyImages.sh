#!/bin/bash

set -euo pipefail

rm -rf public/images
cp -r ../docs/images public/images
