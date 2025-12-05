#!/bin/bash

# Set filenames
CA_KEY="ca.key"
CA_CERT="ca.crt"
SERVER_KEY="server.key"
SERVER_CSR="server.csr"
SERVER_CERT="server.crt"
CA_CONF="ca.conf"
SERVER_CONF="server.conf"
V3_EXT_CONF="v3_ext.conf"

# Generate the CA key
openssl genpkey -algorithm RSA -out $CA_KEY -pkeyopt rsa_keygen_bits:2048

# Create the CA configuration file
cat >$CA_CONF <<EOL
[ req ]
distinguished_name = req_distinguished_name
x509_extensions = v3_ca
prompt = no

[ req_distinguished_name ]
C = ZA
ST = WC
L = Cape Town
O = Coder
OU = Team Coconut
CN = Coder CA

[ v3_ca ]
basicConstraints = critical,CA:TRUE
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer:always
EOL

# Generate the CA certificate
openssl req -new -x509 -key $CA_KEY -out $CA_CERT -days 3650 -config $CA_CONF -extensions v3_ca

# Generate the server key
openssl genpkey -algorithm RSA -out $SERVER_KEY -pkeyopt rsa_keygen_bits:2048

# Create the server configuration file
cat >$SERVER_CONF <<EOL
[ req ]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[ req_distinguished_name ]
C = ZA
ST = WC
L = Cape Town
O = Coder
OU = Team Coconut
CN = myserver.local

[ v3_req ]
subjectAltName = @alt_names

[ alt_names ]
DNS.1 = myserver.local
DNS.2 = www.myserver.local
IP.1 = 127.0.0.1
EOL

# Generate the server CSR
openssl req -new -key $SERVER_KEY -out $SERVER_CSR -config $SERVER_CONF

# Create the server extensions configuration file
cat >$V3_EXT_CONF <<EOL
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names

[ alt_names ]
DNS.1 = myserver.local
DNS.2 = www.myserver.local
IP.1 = 127.0.0.1
EOL

# Generate the server certificate signed by the CA with a validity of 825 days
openssl x509 -req -in $SERVER_CSR -CA $CA_CERT -CAkey $CA_KEY -CAcreateserial -out $SERVER_CERT -days 825 -extfile $V3_EXT_CONF

# Verify the server certificate
openssl x509 -in $SERVER_CERT -text -noout | grep -A 1 "Subject Alternative Name"

echo "CA and server certificates generated successfully."
