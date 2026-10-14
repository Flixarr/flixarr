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

**Contributors.** Settings → About lists the people who worked on Flixarr on GitHub. Flixarr gets that list from the data server (which reads it from GitHub and keeps it for a few hours); the request sends nothing but the Flixarr version in the `User-Agent`.

Never sent: your users, usernames or emails, your requests, your Plex server details, or your library.

### plex.tv

- **Signing in with Plex:** the person approves the sign-in on `app.plex.tv`, and Flixarr reads their Plex account (username, email, picture) from `plex.tv`.
- **Your Plex server:** the owner's Plex account is used to list the Plex servers on it, to see who the server is shared with (who may sign in, and Import Plex users), and to connect. After that, Flixarr reads your libraries from your own Plex server directly.
- **Plex Watchlists:** for people who turn on requests from their Watchlist, Flixarr reads their Watchlist from `discover.provider.plex.tv` with their own Plex sign-in.

### Your own services

Radarr, Sonarr, Maintainerr and your Plex server are contacted at the addresses you enter, and nowhere else.

### Notifications (only what you set up)

Settings → Notifications sends messages only to what an admin turns on: Discord or other webhooks at the addresses entered there, email through the mail server entered there, and push notifications through each browser's own push service (for example Google's or Apple's) for people who allow them. A message names the title and the person it's about.

### Images

By default, posters and backgrounds load from `image.tmdb.org` directly in each viewer's browser, so TMDB sees the viewer's IP address when an image loads. With **Settings → Cache → Cache images** on, your server downloads each image once and serves it to viewers, so TMDB only sees your server.

**Trailers** play in YouTube's privacy-enhanced player (`youtube-nocookie.com`), and only when someone presses Trailer, so YouTube sees the viewer's IP address then. **Contributor pictures** on Settings → About load from GitHub (`avatars.githubusercontent.com`) in the admin's browser.

Movie and series results from the data server are saved in your Flixarr database (**Settings → Cache**) and never sent anywhere else.

That's the full list. Flixarr has no other analytics or telemetry, and its pages tell proxies such as Cloudflare not to inject their own analytics scripts.
