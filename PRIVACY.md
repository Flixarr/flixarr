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

Never sent: your users, usernames or emails, your requests, your Plex server details, or your library.

### plex.tv

Contacted when someone signs in with Plex, and (in later versions) to read your Plex server's libraries.

### Images

Posters and backgrounds load from `image.tmdb.org` directly in each viewer's browser, so TMDB sees the viewer's IP address when an image loads.

That's the full list. Flixarr has no other analytics or telemetry.
