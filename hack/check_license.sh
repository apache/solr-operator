#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

licRes=$(
    find . -type f -iname '*.go' ! -exec \
         sh -c 'head -n5 $1 | grep -Eq "(Licensed to the Apache Software Foundation)" || echo -e  $1' {} {} \;
)

if [ -n "${licRes}" ]; then
	echo -e "license header checking failed:\\n${licRes}"
	exit 255
fi
