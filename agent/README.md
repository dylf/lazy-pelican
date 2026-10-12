# lazy-pelican-agent (experimental)

Starts a stopped Pelican game server when a player tries to join it. It is the "wake" half of Lazy Pelican; the
plugin's idle shutdown is the "sleep" half.

```
player ──UDP──> game port (nothing listening: server is stopped)
                   │  lazy-pelican-agent sees the packets on the host's network interface
                   ▼
        looks like a join attempt?  ──yes──>  panel: is it offline?  ──yes──>  start it
```

The first attempt will still time out, because the game takes a minute or more to boot. Players retry. Once it's up,
the plugin's idle shutdown stops it again after nobody has been on for a while.

## How it decides

- Steam server queries (packets starting `FF FF FF FF`) are ignored. They are what scanners and server browsers send.
- A player's client retries for several seconds, a scanner usually sends one probe. So **3 non-query UDP packets from
  one address within 5 seconds** counts as a join attempt. All three numbers are configurable (in the panel).
- It only starts a server the panel reports as **offline**, and asks at most once every 30 seconds per server.
- After a start it ignores that server for a cooldown (default 5 minutes), and never starts one more than
  `max_wakes_per_hour` times (default 6), so a scanner can't make a server start over and over.
- The panel's own address is never counted, so Player Counter queries can't wake anything.
- If it can't read the server's state, it does nothing. If it can't reach the panel, it keeps watching with the last
  config it got, and retries every 10 seconds until the panel is back.
- Optional per-server match rules (payload prefix, exact length), set on the server's Lazy Pelican page in the panel,
  narrow it further. Real captures of Palworld and Valheim joins are the test fixtures under
  `internal/detector/testdata`.

## What the panel provides

The agent asks the panel (using its API key) for everything except how to reach it:

- **Which servers to watch and on which ports**: the servers with "Wake on connect" turned on, that this key's user may
  start, optionally only those on this agent's `node`. Refreshed every minute, so changes apply without a restart.
- **The detector and limit settings**, set once in the plugin's settings.

It also reports back every 30 seconds: its version, whether packet capture works (and why not), and when it last saw a
join attempt or woke a server. **Admin > Lazy Pelican** in the panel shows this, so a broken agent is visible there
instead of only in a log on the host. If packet capture fails the agent keeps running and retries every 30 seconds.

Anything in the local config file overrides the panel. If the file lists `servers:`, the panel's list is ignored
completely.

## Setup

1. **Panel:** create a user for the agent, add it as a subuser on each server with only the **Start** permission,
   and create a client API key for it (Account > API Keys, starts with `pacc_`).
2. **Config:** copy `deploy/config.example.yml` to `/etc/lazy-pelican-agent/config.yml`. All it needs is the panel URL and the
   node it runs on; the key goes in the `LAZY_PELICAN_API_KEY` environment variable.
3. **Check it:** `lazy-pelican-agent doctor -config /etc/lazy-pelican-agent/config.yml` verifies the key, the permission, packet
   capture, the panel address and the servers the panel wants watched, without starting anything.
4. **Turn on "Wake on connect"** for a server on its Lazy Pelican page in the panel.
5. **Try it safely:** `lazy-pelican-agent run -dry-run -v` logs what it would start but never starts anything.
6. **Router:** forward each game port (UDP) to the Wings host. Nothing about this goes through your reverse proxy.

Run it on the **Docker host next to Wings**, not inside the Wings container. It watches the host's network
interface, which is the same whether Wings is a systemd service or a container.

### Bare metal (systemd)

From the extracted release directory (the binary, `lazy-pelican-agent.service` and `install.sh` are in it), as root:

```bash
sudo PANEL_URL=https://panel.example.com NODE=my-node ./install.sh
```

It asks for the API key (the prompt is silent, so the key never appears on screen or in your shell history), installs
the binary, config, key file (mode 600) and unit, runs `doctor`, then enables and starts the service. Anything not given
as a variable is asked for, `IGNORE_SOURCES=10.0.0.50` adds the panel's LAN address, and it never overwrites an
existing config or key, so running it again upgrades the binary. `NO_START=1` installs without starting.

Or by hand:

```bash
sudo install -m 755 lazy-pelican-agent /usr/local/bin/lazy-pelican-agent
sudo install -d /etc/lazy-pelican-agent
sudo cp deploy/config.example.yml /etc/lazy-pelican-agent/config.yml        # then edit it
echo 'LAZY_PELICAN_API_KEY=pacc_...' | sudo install -m 600 /dev/stdin /etc/lazy-pelican-agent/env
sudo cp deploy/lazy-pelican-agent.service /etc/systemd/system/
sudo systemctl enable --now lazy-pelican-agent
```

The unit runs as a throwaway user with only `CAP_NET_RAW`.

### Docker

See `deploy/docker-compose.example.yml`. It needs `network_mode: host` and `cap_add: [NET_RAW]`, nothing else.
Releases publish `ghcr.io/dylf/lazy-pelican-agent` (amd64 and arm64); to build it yourself, run
`docker build -t lazy-pelican-agent agent/`.

### Releases

Each release attaches `lazy-pelican-agent_<version>_linux_<amd64|arm64>.tar.gz` (the binary plus the example config, systemd
unit, compose file and `install.sh`) and `SHA256SUMS`. Check a download with `sha256sum -c SHA256SUMS --ignore-missing`.

## Status
Tested:
- Real packet captures of join attempts to stopped Valheim and Palworld servers, replayed through the detector.
- Dry run and two real wakes of a Valheim server on a real Wings host, including a player getting in and idle
  shutdown stopping it afterwards.
- A flood of queries, single probes and unwatched ports against the real capture code, none of which start anything.

**Not tested yet:**
- **A Start-only user.** Every real run used the panel owner's key. The setup in the main README is the intended one, but a
  Start-only subuser has not yet been used to start a server.
- A real wake of Palworld (only a dry run), the Docker image, and the systemd unit.
- Players from outside your network, other platforms or game versions, and real internet scanners.

## Limits

- Linux only, IPv4 only, UDP only (first pass).
- It copies every inbound IPv4 packet on the interface to userspace and filters there. That's fine for a game host;
  a BPF filter would be cheaper on a very busy one.
- Games that join through a relay or peer-to-peer (e.g. Valheim crossplay) send nothing to your port, so they can't
  be woken this way. A stopped server also isn't in any server list, so players must join by address.
- Detection is built from captures of two games on one client. Other platforms or game versions may differ; the
  generic rule is the fallback, per-game `match` rules are optional.

## Development

```bash
cd agent
go vet ./... && go test ./...
go build -o lazy-pelican-agent ./cmd/lazy-pelican-agent
```
