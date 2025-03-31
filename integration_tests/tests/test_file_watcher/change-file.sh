#!/bin/bash

E_OPTERR=65

usage() {
    echo "Usage $0 -h | -f <file>"
}
if [ "$#" -eq 0 ]; then   # Script needs at least one command-line argument.
    usage
    exit $E_OPTERR
fi 

set -- `getopt "f:h" "$@"`
while [ ! -z "$1" ]; do
    case "$1" in
        -f)
            filename="$2"
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

if [ -f "$filename" ]; then
    date >> "$filename"
else
    touch "$filename"
fi
