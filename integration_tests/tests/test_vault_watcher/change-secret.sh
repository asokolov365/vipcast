#!/bin/bash

E_OPTERR=65

usage() {
    echo "Usage $0 -h | -p <path> -k <key> -v <value>"
}
if [ "$#" -eq 0 ]; then   # Script needs at least one command-line argument.
    usage
    exit $E_OPTERR
fi 

set -- `getopt "p:k:v:h" "$@"`
while [ ! -z "$1" ]; do
    case "$1" in
        -p)
            path="$2"
            shift
            ;;
        -k)
            key="$2"
            shift
            ;;
        -v)
            value="$2"
            shift
            ;;
        -h)
            usage
            exit 0
            ;;
        *)
            break
            ;;
    esac
    shift
done
if [ -z "$path" -o -z "$key" -o -z "$value" ]; then
    usage
    exit $E_OPTERR
fi

curl --fail --silent \
    --output /dev/null \
    --header "X-Vault-Token: ${VAULT_TOKEN}" \
    --header "Content-Type: application/json" \
    --request POST \
    --data "{\"data\":{\"${key}\":\"${value}\"}}" \
    ${VAULT_ADDR}/v1/${path} \
&& echo "Secret at ${path}[${key}] changed succesfully"
