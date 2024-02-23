#!/usr/bin/env bash

set -exuo pipefail

export AUTHD_SKIP_EXTERNAL_DEPENDENT_TESTS=1
export GOPROXY=off
export GOTOOLCHAIN=local

go test -v ./...
