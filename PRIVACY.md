# Privacy

Flixarr runs on your own server. Your users, requests, library and settings stay in your Flixarr database and are never sent anywhere else.

## What leaves your server

### The Flixarr data server (`api.flixarr.app`)

**Movie and series lookups.** To show titles, Flixarr asks the data server for the IDs you open, the lists you browse, and your search text, along with your region and language and the Flixarr version (in the `User-Agent`). The data server counts these lookups across *all* installs: what kind of lookup, which title or list, and the Flixarr version. It does **not** store your search text, your IP address, or anything that ties a lookup to your install.

**The daily update check.** Once a day, Flixarr asks whether a newer build exists. That check sends exactly:

| Field | Example |
|---|---|
| `install` | a random ID, made once for your server and used for nothing else |
| `version`, `build`, `channel` | `0.1.0`, `42`, `nightly` |
| `os`, `arch` | `linux`, `arm64` |
| `db` | `sqlite` |

The data server keeps these checks so the number of active installs, the versions in use and the hardware they run on can be counted. Admins can see the exact data in **Settings → About → Show exactly what's sent**.

**Turning it off:** switch off **Check for updates daily** in **Settings → About**, or set `FLIXARR_UPDATE_CHECK=false` on the server. When it's off, nothing is sent and you won't hear about updates.

**Quality profiles.** Once a day (and when the update check says there's a new version), Flixarr downloads its quality profiles from the data server. That request sends nothing but the Flixarr version in the `User-Agent`. The profiles are public: they're kept at [Flixarr/flixarr-profiles](https://github.com/Flixarr/flixarr-profiles). Flixarr then adds or updates them in your own Radarr and Sonarr; nothing about your servers is sent anywhere.

Never sent: your users, usernames or emails, your requests, your Plex server details, or your library.

### plex.tv

Contacted when someone signs in with Plex, and (in later versions) to read your Plex server's libraries.

### Images

By default, posters and backgrounds load from `image.tmdb.org` directly in each viewer's browser, so TMDB sees the viewer's IP address when an image loads. With **Settings → Cache → Cache images** on, your server downloads each image once and serves it to viewers, so TMDB only sees your server.

Movie and series results from the data server are saved in your Flixarr database (**Settings → Cache**) and never sent anywhere else.

That's the full list. Flixarr has no other analytics or telemetry, and its pages tell proxies such as Cloudflare not to inject their own analytics scripts.
