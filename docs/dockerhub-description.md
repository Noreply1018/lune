# Lune

Lune is a personal-first LLM gateway with an OpenAI-compatible API, a built-in admin UI, SQLite persistence, and support for both direct provider accounts and CPA-backed CLI accounts.

## Highlights

- OpenAI-compatible gateway for `/v1/*` and `/openai/v1/*`
- Pool-based routing across direct provider accounts and CPA services
- Embedded admin UI for account, pool, token, and settings management
- SQLite persistence in a single self-hosted container workflow
- Prebuilt multi-arch images on both GHCR and Docker Hub

## Quick start

```bash
curl -O https://raw.githubusercontent.com/Noreply1018/lune/main/docker-compose.prod.yml
curl -O https://raw.githubusercontent.com/Noreply1018/lune/main/.env.example
curl -O https://raw.githubusercontent.com/Noreply1018/lune/main/cpa-config.yaml

cp .env.example .env

docker compose -f docker-compose.prod.yml --env-file .env up -d
```

By default the production compose file pulls from GHCR:

```env
LUNE_IMAGE=ghcr.io/noreply1018/lune
LUNE_IMAGE_TAG=latest
```

To pull from Docker Hub instead, set:

```env
LUNE_IMAGE=docker.io/noreply1018/lune
LUNE_IMAGE_TAG=latest
```

## Links

- GitHub: https://github.com/Noreply1018/lune
- Releases: https://github.com/Noreply1018/lune/releases
- README: https://github.com/Noreply1018/lune#readme
