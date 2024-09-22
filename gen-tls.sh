#!/bin/bash

set -e  # Exit immediately if a command exits with a non-zero status

# Generate CA key `ca.key` and cert `ca.crt`
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -subj "/CN=ResourcesShieldCA" -days 1825 -out ca.crt

# Generate server key `server.key` and CSR `server.csr`
openssl genrsa -out server.key 2048
openssl req -new -key server.key -subj "/CN=resources-shield-svc.resources-shield.svc" -out server.csr

# Sign the server cert `server.crt` with the CA cert and create serial number `ca.srl`
openssl x509 -req -in server.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days 1825 \
  -extensions v3_req \
  -extfile <(printf "[v3_req]\\nsubjectAltName=DNS:resources-shield-svc.resources-shield.svc")


# prepare the validation webhook configuration caBundle
CA_BUNDLE=$(openssl base64 -A -in ca.crt)
sed -i '' "s|^\([[:space:]]*caBundle:\).*|\1 $CA_BUNDLE|" manifests/vwc.yaml


# prepare the tls secret
TLS_CRT=$(openssl base64 -A -in server.crt)
TLS_KEY=$(openssl base64 -A -in server.key)
sed -i '' "s|^\([[:space:]]*tls.crt:\).*|\1 $TLS_CRT|" manifests/tls-secret.yaml
sed -i '' "s|^\([[:space:]]*tls.key:\).*|\1 $TLS_KEY|" manifests/tls-secret.yaml


# Clean up generated files
rm -f ca.key ca.crt ca.srl server.key server.csr server.crt

# Optionally print a message indicating cleanup is complete
echo "Cleaned up all generated files."
