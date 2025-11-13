#!/bin/bash

set -e

if [ $# -lt 2 ]; then
    echo "at least 2 arguments required"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

tmpDir=$1
shift

versions=("$@")
versions_without_last=("${versions[@]:0:${#versions[@]}-1}")
last_version="${@: -1}"


cd $tmpDir
cp ${SCRIPT_DIR}/fake-terraform.go .

## Prepare main index.json
# Prefix
cat > ./index.json << EOF
{
  "name": "terraform",
  "versions": {
EOF

# versions with comma in the end
for ver in "${versions_without_last[@]}"; do
  cat >> ./index.json << EOF
    "${ver}": {
      "builds": [
        {
          "arch": "amd64",
          "filename": "terraform_${ver}_linux_amd64.zip",
          "name": "terraform",
          "os": "linux",
          "url": "/terraform/${ver}/terraform_${ver}_linux_amd64.zip",
          "version": "${ver}"
        }
      ],
      "name": "terraform",
      "version": "${ver}"
    },
EOF
done

# last without comma
cat >> ./index.json << EOF
    "${last_version}": {
      "builds": [
        {
          "arch": "amd64",
          "filename": "terraform_${last_version}_linux_amd64.zip",
          "name": "terraform",
          "os": "linux",
          "url": "/terraform/${last_version}/terraform_${last_version}_linux_amd64.zip",
          "version": "${last_version}"
        }
      ],
      "name": "terraform",
      "version": "${last_version}"
    }
  }
}
EOF

## Prepare per version index.json
# Prepare index.json
for ver in "${versions[@]}"; do
  mkdir ./${ver}
  cat > ./${ver}/index.json << EOF
{
  "builds": [
    {
      "arch": "amd64",
      "filename": "terraform_${ver}_linux_amd64.zip",
      "name": "terraform",
      "os": "linux",
      "url": "/terraform/${ver}/terraform_${ver}_linux_amd64.zip",
      "version": "${ver}"
    }
  ],
  "name": "terraform",
  "version": "${ver}"
}
EOF
done

## Prepare zips
go mod init fake-terraform

echo "LICENSE" > ./LICENSE.txt
for ver in "${versions[@]}"; do
  go build -ldflags "-X main.version=${ver}" -o terraform
  zip terraform_${ver}_linux_amd64.zip terraform LICENSE.txt
  mv terraform_${ver}_linux_amd64.zip ./${ver}
done

rm -f LICENSE.txt
rm -f terraform
