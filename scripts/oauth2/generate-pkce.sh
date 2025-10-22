#!/bin/bash

# Generate PKCE code verifier and challenge for OAuth2 flow
# Usage: ./generate-pkce.sh

# Generate code verifier (43-128 characters, URL-safe)
CODE_VERIFIER=$(openssl rand -base64 32 | tr -d "=+/" | cut -c -43)

# Generate code challenge (S256 method)
CODE_CHALLENGE=$(echo -n "$CODE_VERIFIER" | openssl dgst -sha256 -binary | base64 | tr -d "=" | tr '+/' '-_')

echo "Code Verifier: $CODE_VERIFIER"
echo "Code Challenge: $CODE_CHALLENGE"

# Export as environment variables for use in other scripts
export CODE_VERIFIER
export CODE_CHALLENGE

echo ""
echo "Environment variables set:"
echo "  CODE_VERIFIER=\"$CODE_VERIFIER\""
echo "  CODE_CHALLENGE=\"$CODE_CHALLENGE\""
echo ""
echo "Usage in curl:"
echo "  curl \"...&code_challenge=$CODE_CHALLENGE&code_challenge_method=S256\""
echo "  curl -d \"code_verifier=$CODE_VERIFIER\" ..."
