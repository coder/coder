#!/usr/bin/env bash

# Add the cross-sign issuing certificates from the subordinate certificate
# authorities.
# See: https://learn.microsoft.com/en-us/azure/security/fundamentals/azure-ca-details
declare -a CERTIFICATES=(
	"Microsoft Azure ECC TLS Issuing CA 03=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20ECC%20TLS%20Issuing%20CA%2003%20-%20xsign.crt"
	"Microsoft Azure ECC TLS Issuing CA 04=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20ECC%20TLS%20Issuing%20CA%2004%20-%20xsign.crt"
	"Microsoft Azure ECC TLS Issuing CA 07=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20ECC%20TLS%20Issuing%20CA%2007%20-%20xsign.crt"
	"Microsoft Azure ECC TLS Issuing CA 08=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20ECC%20TLS%20Issuing%20CA%2008%20-%20xsign.crt"
	"Microsoft Azure RSA TLS Issuing CA 03=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20RSA%20TLS%20Issuing%20CA%2003%20-%20xsign.crt"
	"Microsoft Azure RSA TLS Issuing CA 04=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20RSA%20TLS%20Issuing%20CA%2004%20-%20xsign.crt"
	"Microsoft Azure RSA TLS Issuing CA 07=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20RSA%20TLS%20Issuing%20CA%2007%20-%20xsign.crt"
	"Microsoft Azure RSA TLS Issuing CA 08=https://www.microsoft.com/pkiops/certs/Microsoft%20Azure%20RSA%20TLS%20Issuing%20CA%2008%20-%20xsign.crt"

	# Azure IMDS G2 attested data chains can use the cross-signed
	# Microsoft TLS RSA Root G2 to sign these OCSP intermediates.
	"Microsoft TLS RSA Root G2=https://www.microsoft.com/pkiops/certs/Microsoft%20TLS%20RSA%20Root%20G2%20-%20xsign.crt"
	"Microsoft TLS G2 RSA CA OCSP 02=https://www.microsoft.com/pkiops/certs/Microsoft%20TLS%20G2%20RSA%20CA%20OCSP%2002.crt"
	"Microsoft TLS G2 RSA CA OCSP 04=https://www.microsoft.com/pkiops/certs/Microsoft%20TLS%20G2%20RSA%20CA%20OCSP%2004.crt"
	"Microsoft TLS G2 RSA CA OCSP 06=https://www.microsoft.com/pkiops/certs/Microsoft%20TLS%20G2%20RSA%20CA%20OCSP%2006.crt"
	"Microsoft TLS G2 RSA CA OCSP 08=https://www.microsoft.com/pkiops/certs/Microsoft%20TLS%20G2%20RSA%20CA%20OCSP%2008.crt"
	"Microsoft TLS G2 RSA CA OCSP 10=https://www.microsoft.com/pkiops/certs/Microsoft%20TLS%20G2%20RSA%20CA%20OCSP%2010.crt"
	"Microsoft TLS G2 RSA CA OCSP 12=https://www.microsoft.com/pkiops/certs/Microsoft%20TLS%20G2%20RSA%20CA%20OCSP%2012.crt"
	"Microsoft TLS G2 RSA CA OCSP 14=https://www.microsoft.com/pkiops/certs/Microsoft%20TLS%20G2%20RSA%20CA%20OCSP%2014.crt"
	"Microsoft TLS G2 RSA CA OCSP 16=https://www.microsoft.com/pkiops/certs/Microsoft%20TLS%20G2%20RSA%20CA%20OCSP%2016.crt"

	# These have expired, but leaving them in for now.
	"Microsoft RSA TLS CA 01=https://crt.sh/?d=3124375355"
	"Microsoft RSA TLS CA 02=https://crt.sh/?d=3124375356"
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
