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

## Wake on connect (experimental)

> **Experimental.** The agent has been tested on one Wings host with Valheim and Palworld, from one client machine.
> Expect rough edges, and read "Status" below before relying on it.

Idle Shutdown only stops servers. Starting one again when a player tries to join needs a small separate program, the
**lazy-agent**, running on the Wings host, because the panel can't see network traffic. It is not part of the plugin
zip and the plugin works fine without it. If you don't use it, friends with the Start permission can press Start in
the panel instead.

```
player ──UDP──> game port (nothing listening: server is stopped)
                   │  lazy-agent sees the packets on the host's network interface
                   ▼
        looks like a join attempt?  ──yes──>  panel: is it offline?  ──yes──>  start it
```

The first attempt always times out, because the game takes a minute or more to boot. Players retry. A stopped server
also isn't in any in-game server list, so players join by address. Once it's up, Idle Shutdown stops it again after
nobody has been on for a while.

### What you need
- Idle Shutdown already working on the servers you want (see Setup above).
- A Linux Wings host. The agent runs **on the Docker host next to Wings** (not inside the Wings container), whether
  Wings itself runs under systemd or in Docker. It needs `CAP_NET_RAW` to read packets, nothing else.
- UDP port forwards from your router to the Wings host for each game port. The agent does not go through a reverse
  proxy, and neither does game traffic.

### Setup
1. **Create a user for the agent** in the panel (a normal user, **not** an admin). Add it as a subuser on each server
   you want woken, with only the **Control > Start** permission.
2. **Create a client API key as that user.** Log in as the agent's user, open the account menu, then **API Keys**.
   This is the account (client) key, which starts with `pacc_`. It is *not* the admin "Application" key under
   Admin > API Keys, which doesn't work for this. The key is shown once.
3. **Get the binary** and the example config from the `agent/` directory (build with
   `cd agent && go build -o lazy-agent ./cmd/lazy-agent`; there are no published releases yet).
4. **Write the config** (copy `agent/deploy/config.example.yml`). For each server you need its short identifier, from
   the console URL `https://panel/server/<id>/console`, and its game port(s) as players connect to them:
   ```yaml
   panel_url: https://panel.example.com
   ignore_sources: [10.0.0.112]     # the panel's own LAN address, if the panel is on your network
   servers:
     - {name: valheim,  id: dc93fa79, ports: [27000]}   # the game port, not the query port
     - {name: palworld, id: 60455931, ports: [27002]}
   ```
   Give the key through the environment (`LAZY_AGENT_API_KEY`), not the file.
5. **Check it:** `lazy-agent doctor -config config.yml`. It verifies the key, the Start permission, packet capture
   and the panel address without starting anything.
6. **Dry run:** `lazy-agent run -config config.yml -dry-run -v`. Try to join a stopped server and watch for
   `dry run: would start the server`. Nothing is started.
7. **Run it for real**, as a systemd service or a container. See [agent/README.md](agent/README.md) for the unit
   file, the compose example and the details.

### How it decides
- Steam server queries (packets starting `FF FF FF FF`) are ignored. They're what scanners and server browsers send.
- A player's client retries for several seconds, while a scanner usually sends one probe. So **3 non-query UDP packets
  from one address within 5 seconds** counts as a join attempt (all three numbers are configurable).
- It only starts a server the panel reports as **offline**.
- After a start it leaves that server alone for a cooldown (default 5 minutes) and never starts one more than 6 times an
  hour (default), so a scanner can't make a server start over and over.
- If it can't read a server's state, it does nothing.

### Status
Tested:
- Real packet captures of join attempts to stopped Valheim and Palworld servers, replayed through the detector.
- Dry run and two real wakes of a Valheim server on a real Wings host, including a player getting in and Idle
  Shutdown stopping it afterwards.
- A flood of queries, single probes and unwatched ports against the real capture code, none of which start anything.

**Not tested yet:**
- **A Start-only user.** Every real run used the panel owner's key. The setup above is the intended one, but a
  Start-only subuser has not yet been used to start a server.
- A real wake of Palworld (only a dry run), the Docker image, and the systemd unit.
- Players from outside your network, other platforms or game versions, and real internet scanners.

### Limits
- Linux only, IPv4 only, UDP only.
- Servers and ports are listed by hand in the config. There is no discovery from the panel yet.
- Games that join through a relay or peer to peer (such as Valheim crossplay) send nothing to your port, so they can't
  be woken.
- Valheim must have **Public Server** on for Idle Shutdown to read its player count (see Games above). That also lists
  the server publicly while it's running.

## Config (env)
| Variable | Default |
|---|---|
| `LAZY_PELICAN_DEFAULT_IDLE_MINUTES` | 15 |
| `LAZY_PELICAN_DEFAULT_GRACE_MINUTES` | 10 |
| `LAZY_PELICAN_VERBOSE` | false |

