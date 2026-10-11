# lazy-agent (experimental)

Starts a stopped Pelican game server when a player tries to join it. It is the "wake" half of Lazy Pelican; the
plugin's Idle Shutdown is the "sleep" half.

```
player ──UDP──> game port (nothing listening: server is stopped)
                   │  lazy-agent sees the packets on the host's network interface
                   ▼
        looks like a join attempt?  ──yes──>  panel: is it offline?  ──yes──>  start it
```

The first attempt will still time out, because the game takes a minute or more to boot. Players retry. Once it's up,
the plugin's Idle Shutdown stops it again after nobody has been on for a while.

## How it decides

- Steam server queries (packets starting `FF FF FF FF`) are ignored. They are what scanners and server browsers send.
- A player's client retries for several seconds, a scanner usually sends one probe. So **3 non-query UDP packets from
  one address within 5 seconds** counts as a join attempt. All three numbers are configurable.
- It only starts a server the panel reports as **offline**, and asks at most once every 30 seconds per server.
- After a start it ignores that server for a cooldown (default 5 minutes), and never starts one more than
  `max_wakes_per_hour` times (default 6), so a scanner can't make a server start over and over.
- The panel's own address is never counted, so Player Counter queries can't wake anything.
- If it can't read the server's state, it does nothing.
- Optional per-server `match` rules (payload prefix, exact length) narrow it further. Real captures of Palworld and
  Valheim joins are the test fixtures under `internal/detector/testdata`.

## Setup

1. **Panel:** create a user for the agent, add it as a subuser on each server with only the **Start** permission,
   and create a client API key for it (Account > API Keys).
2. **Config:** copy `deploy/config.example.yml` to `/etc/lazy-agent/config.yml` and fill in the panel URL and servers.
3. **Check it:** `lazy-agent doctor -config /etc/lazy-agent/config.yml` verifies the key, the permission, packet
   capture and the panel address, without starting anything.
4. **Try it safely:** `lazy-agent run -dry-run -v` logs what it would start but never starts anything.
5. **Router:** forward each game port (UDP) to the Wings host. Nothing about this goes through your reverse proxy.

Run it on the **Docker host next to Wings**, not inside the Wings container. It watches the host's network
interface, which is the same whether Wings is a systemd service or a container.

### Bare metal (systemd)

```bash
sudo install -m 755 lazy-agent /usr/local/bin/lazy-agent
sudo install -d /etc/lazy-agent
sudo cp deploy/config.example.yml /etc/lazy-agent/config.yml        # then edit it
echo 'LAZY_AGENT_API_KEY=pacc_...' | sudo install -m 600 /dev/stdin /etc/lazy-agent/env
sudo cp deploy/lazy-agent.service /etc/systemd/system/
sudo systemctl enable --now lazy-agent
```

The unit runs as a throwaway user with only `CAP_NET_RAW`.

### Docker

See `deploy/docker-compose.example.yml`. It needs `network_mode: host` and `cap_add: [NET_RAW]`, nothing else.
Build with `docker build -t lazy-pelican-agent agent/`.

## Limits

- Linux only, IPv4 only, UDP only (first pass).
- It copies every inbound IPv4 packet on the interface to userspace and filters there. That's fine for a game host;
  a BPF filter would be cheaper on a very busy one.
- It has to be told which servers and ports to watch (no discovery from the panel yet).
- Games that join through a relay or peer-to-peer (e.g. Valheim crossplay) send nothing to your port, so they can't
  be woken this way. A stopped server also isn't in any server list, so players must join by address.
- Detection is built from captures of two games on one client. Other platforms or game versions may differ; the
  generic rule is the fallback, per-game `match` rules are optional.

## Development

```bash
cd agent
go vet ./... && go test ./...
go build -o lazy-agent ./cmd/lazy-agent
```
