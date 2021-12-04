#!/bin/bash

CUSTOM_GOOS=$1
CUSTOM_GOARCH=$2

if [[ "$CUSTOM_GOOS" != "" ]]; then
  export GOOS="$CUSTOM_GOOS"
fi

if [[ "$CUSTOM_GOARCH" != "" ]]; then
  export GOARCH="$CUSTOM_GOARCH"
fi

if [[ "$3" == "-o" ]]; then
  export OUTPUT_ARG="-o $4"
fi

export CGO_ENABLED=0

LDFlags="\
    -s -w
"

go build ${OUTPUT_ARG} -trimpath -ldflags "${LDFlags}"
