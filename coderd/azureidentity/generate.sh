#!/usr/bin/env bash

# See: https://learn.microsoft.com/en-us/azure/security/fundamentals/azure-ca-details?tabs=certificate-authority-chains
declare -a CERTIFICATES=(
	"Microsoft RSA TLS CA 01=https://crt.sh/?d=3124375355"
	"Microsoft RSA TLS CA 02=https://crt.sh/?d=3124375356"
	"Microsoft Azure RSA TLS Issuing CA 03=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20RSA%20TLS%20Issuing%20CA%2003%20-%20xsign.crt"
	"Microsoft Azure RSA TLS Issuing CA 04=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20RSA%20TLS%20Issuing%20CA%2004%20-%20xsign.crt"
	"Microsoft Azure RSA TLS Issuing CA 07=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20RSA%20TLS%20Issuing%20CA%2007%20-%20xsign.crt"
	"Microsoft Azure RSA TLS Issuing CA 08=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20RSA%20TLS%20Issuing%20CA%2008%20-%20xsign.crt"
	"Microsoft Azure TLS Issuing CA 01=https://www.microsoft.com/pki/certs/Microsoft%20Azure%20TLS%20Issuing%20CA%2001.cer"
	"Microsoft Azure TLS Issuing CA 02=https://www.microsoft.com/pki/certs/Microsoft%20Azure%20TLS%20Issuing%20CA%2002.cer"
	"Microsoft Azure TLS Issuing CA 05=https://www.microsoft.com/pki/certs/Microsoft%20Azure%20TLS%20Issuing%20CA%2005.cer"
	"Microsoft Azure TLS Issuing CA 06=https://www.microsoft.com/pki/certs/Microsoft%20Azure%20TLS%20Issuing%20CA%2006.cer"
)

CONTENT="var Certificates = []string{"

for CERT in "${CERTIFICATES[@]}"; do
	IFS="=" read -r NAME URL <<<"$CERT"
	echo "Downloading certificate: $NAME"
	PEM=$(curl -sSL "$URL" | openssl x509 -outform PEM)
	echo "$PEM"

	CONTENT+="\n// $NAME\n\`$PEM\`,"
done

CONTENT+="\n}"

sed -i '/var Certificates = /,$d' azureidentity.go
# shellcheck disable=SC2059
printf "$CONTENT" >>azureidentity.go
gofmt -w azureidentity.go
