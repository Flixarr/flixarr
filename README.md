<p align="center"><b>Flixarr</b></p>

Flixarr is a self-hosted app for requesting movies and series for your Plex server: browse what's trending, request it, and track it until it's ready to watch.

> **Early development.** Flixarr is being built in the open and isn't ready for general use yet.

## What works so far

- First-run setup: sign in with Plex, and the first account becomes the admin
- Discover with a rotating spotlight and trending / popular / upcoming rows
- Movie, series (with seasons and episodes), person and collection pages
- Search across movies, series and people
- Browse movies and series by genre

Requests, Radarr/Sonarr, notifications, issues and settings are next.

## Running it

```yaml
services:
  flixarr:
    image: flixarr:local
    ports:
      - "7575:7575"
    volumes:
      - flixarr-data:/data
    restart: unless-stopped
volumes:
  flixarr-data:
```

Build the image with `docker build -t flixarr:local .`, then open `http://<host>:7575`.

| Variable | Default | What |
|---|---|---|
| `FLIXARR_PORT` | `7575` | Port to listen on |
| `FLIXARR_DATA` | `/data` | Where the database and backups live |
| `FLIXARR_API` | `https://api.flixarr.app` | The Flixarr data server |

## Movie and series data

Flixarr gets movie and series information from the Flixarr data server (`api.flixarr.app`), so you never need your own API key. See [PRIVACY.md](PRIVACY.md) for exactly what that server receives.

This product uses the TMDB API but is not endorsed or certified by TMDB.

## Versioning

`MAJOR.MINOR.PATCH.BUILD`. `0.x` while in early development; `1.0.0` will be the first public release. BUILD is a counter that goes up with every build.

## License

To be decided.
