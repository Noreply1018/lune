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
cp .env.example .env

docker compose -f docker-compose.prod.yml --env-file .env up -d
```

The Lune image includes the CPA runtime by default, so no separate CPA image or `cpa-config.yaml` is required. A single Docker volume mounted at `/app/data` stores SQLite data, CPA auth files, and gateway temporary replay files.

Gateway request bodies default to a 100 MB limit. Requests above 8 MB are replayed from disk for retries instead of being kept entirely in memory.

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
