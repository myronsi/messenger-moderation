#!/usr/bin/env bash
set -euo pipefail

dir=${1:?usage: deploy.sh <app dir> <image>}
image=${2:?usage: deploy.sh <app dir> <image>}

cd "$dir"
if [ ! -f .env ]; then
  echo "deploy: $dir/.env is missing; create it from env.example first (docs/deploy.md)" >&2
  exit 1
fi

export MODERATION_IMAGE="$image"
docker compose -f compose.yaml pull moderation
docker compose -f compose.yaml run --rm --no-deps moderation migrate
docker compose -f compose.yaml up -d --wait moderation

if grep -q '^MODERATION_IMAGE=' .env; then
  sed -i "s|^MODERATION_IMAGE=.*|MODERATION_IMAGE=$image|" .env
else
  printf '\nMODERATION_IMAGE=%s\n' "$image" >> .env
fi
echo "deploy: $image is running"
