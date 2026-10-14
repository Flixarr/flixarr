<p align="center"><b>Flixarr</b></p>

Flixarr is a self-hosted app for requesting movies and series for your Plex server: browse what's trending, request it, and track it until it's ready to watch.

> **Early development.** Flixarr is being built in the open and isn't ready for general use yet.

## What works so far

- **Sign in** with Plex or a local account. The first account becomes the owner.
- **Discover**: a rotating spotlight, trending, popular and upcoming rows, Recently Added and Recent Requests from your own server, and a Leaving Soon row. Admins can reorder or hide rows.
- **Movie, series, person and collection pages**, search, and browse by genre with endless scrolling.
- **Requests** for movies and series (whole show or picked seasons), with optional approval, per person request limits, and live status from requested to downloading to available.
- **Radarr and Sonarr**: status cards with queue, disk space and health, alerts when something needs attention, and Fix and Redownload buttons for stuck or bad downloads.
- **Flixarr quality profiles**: ready made Standard, Space Saver and 4K profiles that Flixarr adds to Radarr and Sonarr and keeps up to date.
- **Plex library sync**: titles show as available the moment they reach Plex, and removals show up right away too.
- **Users and permissions**: roles as presets with individual switches, and import of the people your Plex server is shared with.
- **Notifications** through Discord, webhooks, email and web push, chosen per event and per person.
- **Report an issue** with a title (video, audio, subtitles), with comments and resolve.
- **Plex Watchlist requests**, off by default and switched on per person.
- **Maintainerr support**: leaving dates, a heads up before a title goes, and Keep or ask to keep.
- **Backups** with restore, a Jobs page, cache settings, and an installable app for your phone.

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

## Development

Work happens on the `develop` branch, and nightly builds come from it. `main` only moves when a release is published. Bugs and ideas are tracked in [issues](https://github.com/Flixarr/flixarr/issues); see [CONTRIBUTING.md](CONTRIBUTING.md) to help.

## Movie and series data

Flixarr gets movie and series information from the Flixarr data server (`api.flixarr.app`), so you never need your own API key. See [PRIVACY.md](PRIVACY.md) for exactly what that server receives.

This product uses the TMDB API but is not endorsed or certified by TMDB.

## Versioning

`MAJOR.MINOR.PATCH.BUILD`. `0.x` while in early development; `1.0.0` will be the first public release. BUILD is the number of commits on `develop`, so it goes up with every change.

## License

Copyright (C) 2026 Marc Hershey

Flixarr is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License, version 3, as published by the Free Software Foundation. It is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See [LICENSE](LICENSE) for the full text.

The Flixarr name and logo are not covered by this license.
