# Lazy Pelican (Pelican plugin)

Stops a game server automatically after it has had **no players** for a set time.
Starting is manual: players you've added as subusers can press Start in the panel.

## How it works
- Every minute a scheduled command (`lazy-pelican:check`) looks at each server that has idle shutdown enabled.
- Player counts come from the **Player Counter** plugin's Game Query for the server's egg
  (A2S/Source for Valheim, the REST API for Palworld; see Games below).
- Not running -> nothing happens. Running -> a startup grace period, then idle time is counted.
- When the player count has been 0 for the idle time, a normal panel `stop` power action is sent
  (Valheim's egg stops with ^C, so the world is saved).
- Fail-safe: if the player count can't be read, the idle timer resets and the server is **not** stopped.

## Requirements
- Pelican panel with the plugin system
- [player-counter](https://hub.pelican.dev/plugins/player-counter) plugin installed, with a Game Query linked to the egg
- The panel's scheduler and queue worker running (`php artisan schedule:run` via cron)

## Setup
1. Install and enable `player-counter`. Admin -> Game Queries -> create a query for your game (see below) and link it
   to the egg. The seeder does not always create these, so check the list.
2. Install this plugin (import the zip on the Plugins page, or copy folder `lazy-pelican` to `plugins/` and run
   `php artisan p:plugin:install`).
3. Open the server -> **Idle Shutdown**, enable it, set minutes. The page warns you if Player Counter or the
   Game Query is missing.
4. Give subusers the **Idle Shutdown** permission if they should manage it.

## Games
Idle Shutdown works with any game Player Counter can query. Tested so far:

| Game | Game Query | Needs |
|---|---|---|
| Valheim | **Source**, query port offset **1** | **Public Server must be on** (see below) |
| Palworld | **Palworld**, no offset | REST API enabled on the game port, plus an `ADMIN_PASSWORD` (see below) |

If the player count can't be read, nothing is ever stopped, so a game that isn't set up correctly just never shuts
down. It does not get stopped by mistake.

### Valheim: Public Server must be on
The Valheim server only answers the Steam query when `-public 1`. With Public Server off, the player count is
unavailable and Idle Shutdown can never stop the server. With a password set, "public" only means the server is
listed in the in-game browser. Use the **Source** query with offset 1 (game port 27000 -> query port 27001), and make
sure the query port is also an allocation on the server so Docker maps it.

### Palworld: enable the REST API
Player Counter reads Palworld through its REST API (`/v1/api/info`, `/metrics`, `/players`), authenticating as `admin`
with the server's `ADMIN_PASSWORD` variable. The REST API listens on the **same port as the game** (TCP, while the game
is UDP), so Player Counter finds it on the server's allocation.

The stock egg uses RCON and does not enable the REST API, but its config tool reads `REST_API_ENABLED` and
`REST_API_PORT`. Without editing the egg, prefix the server's startup command (admin -> server -> Startup) with:

```
export REST_API_ENABLED=True REST_API_PORT={{SERVER_PORT}}; 
```

Check it works from the panel host: `curl -u admin:<ADMIN_PASSWORD> http://<ip>:<port>/v1/api/info`.
Set a non-empty `ADMIN_PASSWORD` on the server, or the query returns nothing.

## Config (env)
| Variable | Default |
|---|---|
| `LAZY_PELICAN_DEFAULT_IDLE_MINUTES` | 15 |
| `LAZY_PELICAN_DEFAULT_GRACE_MINUTES` | 10 |
| `LAZY_PELICAN_VERBOSE` | false |

