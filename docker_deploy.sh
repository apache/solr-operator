#!/bin/sh

docker login -u "$DOCKER_USER" -p "$DOCKER_PASSWORD" \
    && make docker-push \
    && make docker-vendor-push