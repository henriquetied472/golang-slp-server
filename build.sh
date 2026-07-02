#!/bin/bash

echo "Building Windows/x86-64 executable..."
GOOS=windows go build -o build/switch-lan-play.exe

echo "Building Linux/aarch64 executable..."
GOARCH=arm64 go build -o build/switch-lan-play.aarch64

echo "Building Linux/x86-64 executable..."
go build -o build/switch-lan-play.x86-64

if [ "$1" == "deploy" ]; then
    echo "Deploying Linux/aarch64 to $DEPLOY_SSH"
    rsync -a --force build/switch-lan-play.aarch64 ${DEPLOY_SSH}:${DEPLOY_DEST}
fi
