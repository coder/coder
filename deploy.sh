#!/usr/bin/env bash

VERSION=$(./scripts/version.sh)

FILE=build/coder_${VERSION}_linux_arm64.deb
FLAGS="--project=coder-dogfood --zone=us-central1-b"

make -j $FILE
gcloud compute scp $FLAGS $FILE "kyle@kyle-ai:/tmp/coder.deb"
gcloud compute ssh $FLAGS "kyle@kyle-ai" -- sudo dpkg -i /tmp/coder.deb
gcloud compute ssh $FLAGS "kyle@kyle-ai" -- sudo systemctl daemon-reload
gcloud compute ssh $FLAGS "kyle@kyle-ai" -- sudo systemctl restart coder
