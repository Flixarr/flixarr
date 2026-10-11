# Privacy

Flixarr runs on your own server. Your users, requests, library and settings stay in your Flixarr database and are never sent anywhere else.

## What leaves your server

**The Flixarr data server (`api.flixarr.app`)** receives the lookups needed to show movie and series information:

- the title or person IDs you open, search text, and which lists you browse
- your region and language setting (for release dates and titles)
- your server's IP address (as with any web request) and the Flixarr version in the `User-Agent`

It does **not** receive your users, your requests, your Plex details or your library. Responses are cached at the edge, and the data server keeps no per-install logs beyond Cloudflare's standard request handling.

**plex.tv** is contacted when someone signs in with Plex, and later to read your Plex server's libraries.

**Image hosts:** posters and backgrounds load from `image.tmdb.org` in each viewer's browser.

That's the full list. Flixarr has no analytics or telemetry.
