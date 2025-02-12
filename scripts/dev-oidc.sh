#!/usr/bin/env bash

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
source "${SCRIPT_DIR}/lib.sh"

# Allow toggling verbose output
[[ -n ${VERBOSE:-} ]] && set -x
set -euo pipefail

KEYCLOAK_VERSION="${KEYCLOAK_VERSION:-22.0}"

# NOTE: the trailing space in "lastName" is intentional.
cat <<EOF >/tmp/example-realm.json
{
  "realm": "coder",
  "enabled": true,
  "sslRequired": "none",
  "registrationAllowed": true,
  "privateKey": "MIICXAIBAAKBgQCrVrCuTtArbgaZzL1hvh0xtL5mc7o0NqPVnYXkLvgcwiC3BjLGw1tGEGoJaXDuSaRllobm53JBhjx33UNv+5z/UMG4kytBWxheNVKnL6GgqlNabMaFfPLPCF8kAgKnsi79NMo+n6KnSY8YeUmec/p2vjO2NjsSAVcWEQMVhJ31LwIDAQABAoGAfmO8gVhyBxdqlxmIuglbz8bcjQbhXJLR2EoS8ngTXmN1bo2L90M0mUKSdc7qF10LgETBzqL8jYlQIbt+e6TH8fcEpKCjUlyq0Mf/vVbfZSNaVycY13nTzo27iPyWQHK5NLuJzn1xvxxrUeXI6A2WFpGEBLbHjwpx5WQG9A+2scECQQDvdn9NE75HPTVPxBqsEd2z10TKkl9CZxu10Qby3iQQmWLEJ9LNmy3acvKrE3gMiYNWb6xHPKiIqOR1as7L24aTAkEAtyvQOlCvr5kAjVqrEKXalj0Tzewjweuxc0pskvArTI2Oo070h65GpoIKLc9jf+UA69cRtquwP93aZKtW06U8dQJAF2Y44ks/mK5+eyDqik3koCI08qaC8HYq2wVl7G2QkJ6sbAaILtcvD92ToOvyGyeE0flvmDZxMYlvaZnaQ0lcSQJBAKZU6umJi3/xeEbkJqMfeLclD27XGEFoPeNrmdx0q10Azp4NfJAY+Z8KRyQCR2BEG+oNitBOZ+YXF9KCpH3cdmECQHEigJhYg+ykOvr1aiZUMFT72HU0jnmQe2FVekuG+LJUt2Tm7GtMjTFoGpf0JwrVuZN39fOYAlo+nTixgeW7X8Y=",
  "publicKey": "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCrVrCuTtArbgaZzL1hvh0xtL5mc7o0NqPVnYXkLvgcwiC3BjLGw1tGEGoJaXDuSaRllobm53JBhjx33UNv+5z/UMG4kytBWxheNVKnL6GgqlNabMaFfPLPCF8kAgKnsi79NMo+n6KnSY8YeUmec/p2vjO2NjsSAVcWEQMVhJ31LwIDAQAB",
  "requiredCredentials": ["password"],
  "users": [
    {
      "username": "oidcuser",
      "email": "oidcuser@coder.com",
      "firstName": "OIDC",
      "lastName": "user ",
      "emailVerified": true,
      "enabled": true,
      "credentials": [
        {
          "type": "password",
          "value": "password"
        }
      ],
      "clientRoles": {
        "realm-management": ["realm-admin"],
        "account": ["manage-account"]
      }
    }
  ],
  "clients": [
    {
      "clientId": "coder",
      "directAccessGrantsEnabled": true,
      "enabled": true,
      "fullScopeAllowed": true,
      "baseUrl": "/coder",
      "redirectUris": ["*"],
      "secret": "coder"
    },
    {
      "clientId": "coder-public",
      "publicClient": true,
      "directAccessGrantsEnabled": true,
      "enabled": true,
      "fullScopeAllowed": true,
      "baseUrl": "/coder",
      "redirectUris": [
        "*"
      ]
    }
  ]
}
EOF

echo '== Starting Keycloak'
docker rm -f keycloak || true
# Start Keycloak
docker run --rm -d \
	--name keycloak \
	-p 9080:8080 \
	-e KEYCLOAK_ADMIN=admin \
	-e KEYCLOAK_ADMIN_PASSWORD=password \
	-v /tmp/example-realm.json:/opt/keycloak/data/import/example-realm.json \
	"quay.io/keycloak/keycloak:${KEYCLOAK_VERSION}" \
	start-dev \
	--import-realm

echo '== Waiting for keycloak to become ready'
# Start the timeout in the background so interrupting this script
# doesn't hang for 60s.
timeout 60s bash -c 'until curl -s --fail http://localhost:9080/realms/coder/.well-known/openid-configuration > /dev/null 2>&1; do sleep 0.5; done' ||
	fatal 'Keycloak did not become ready in time' &
wait $!

echo '== Starting Coder'
hostname=$(hostname -f)
export CODER_OIDC_ISSUER_URL="http://${hostname}:9080/realms/coder"
export CODER_OIDC_CLIENT_ID=coder
export CODER_OIDC_CLIENT_SECRET=coder
# Comment out the two lines above, and comment in the line below,
# to configure OIDC auth using a public client.
# export CODER_OIDC_CLIENT_ID=coder-public
export CODER_DEV_ACCESS_URL="http://${hostname}:8080"

exec "${SCRIPT_DIR}/develop.sh" "$@"
