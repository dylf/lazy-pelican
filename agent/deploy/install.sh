#!/usr/bin/env bash
# Installs lazy-pelican-agent as a systemd service, following the steps in the README.
#
# Run it as root from the extracted release directory (it needs lazy-pelican-agent and lazy-pelican-agent.service
# next to it):
#
#   sudo LAZY_PELICAN_API_KEY=pacc_... PANEL_URL=https://panel.example.com NODE=my-node ./install.sh
#
# Anything not given is asked for. It never overwrites an existing config or key file, so it is safe to run again to
# upgrade the binary. Set NO_START=1 to install without enabling or starting the service.
set -euo pipefail

BIN_DIR="${BIN_DIR:-/usr/local/bin}"
CONF_DIR="${CONF_DIR:-/etc/lazy-pelican-agent}"
UNIT_DIR="${UNIT_DIR:-/etc/systemd/system}"
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

# Installing to system directories needs root. LAZY_INSTALL_ROOTLESS is only for testing the script itself.
if [ "$(id -u)" -ne 0 ] && [ -z "${LAZY_INSTALL_ROOTLESS:-}" ]; then
  die "run this as root (sudo)"
fi

# Find the binary: next to this script (release tarball), or one level up (a source checkout where it was built).
BINARY=""
for candidate in "$SRC/lazy-pelican-agent" "$SRC/../lazy-pelican-agent" "./lazy-pelican-agent"; do
  if [ -f "$candidate" ] && [ -x "$candidate" ]; then BINARY="$candidate"; break; fi
done
[ -n "$BINARY" ] || die "can't find the lazy-pelican-agent binary next to this script"

UNIT="$SRC/lazy-pelican-agent.service"
[ -f "$UNIT" ] || die "can't find lazy-pelican-agent.service next to this script"

ask() { # ask VARNAME "prompt" [default] [secret]
  local var="$1" prompt="$2" default="${3:-}" secret="${4:-}" value
  value="${!var:-}"
  if [ -z "$value" ]; then
    if [ ! -t 0 ]; then
      # No terminal to ask on: only a default can fill the gap.
      [ -n "$default" ] || die "$var is not set and there is no terminal to ask on"
      value="$default"
    else
      if [ -n "$secret" ]; then
        read -r -s -p "$prompt: " value
        echo >&2
      else
        read -r -p "$prompt${default:+ [$default]}: " value
      fi
      value="${value:-$default}"
    fi
  fi
  [ -n "$value" ] || die "$var is required"
  printf -v "$var" '%s' "$value"
}

say "Installing lazy-pelican-agent"
install -d "$BIN_DIR" "$CONF_DIR"
install -m 755 "$BINARY" "$BIN_DIR/lazy-pelican-agent"
say "  binary:  $BIN_DIR/lazy-pelican-agent ($("$BIN_DIR/lazy-pelican-agent" version))"

CONFIG="$CONF_DIR/config.yml"
if [ -e "$CONFIG" ]; then
  say "  config:  $CONFIG already exists, leaving it alone"
else
  ask PANEL_URL "Panel URL (e.g. https://panel.example.com)"
  ask NODE "This host's node in the panel (id, name or FQDN)"
  {
    echo "# lazy-pelican-agent configuration. Servers, ports and thresholds come from the panel."
    echo "panel_url: $PANEL_URL"
    echo "node: $NODE"
    if [ -n "${IGNORE_SOURCES:-}" ]; then
      echo "# The panel's own LAN address, so its queries are never counted as players."
      echo "ignore_sources: [$IGNORE_SOURCES]"
    fi
  } >"$CONFIG"
  chmod 644 "$CONFIG"
  say "  config:  $CONFIG written"
fi

ENVFILE="$CONF_DIR/env"
if [ -e "$ENVFILE" ]; then
  say "  key:     $ENVFILE already exists, leaving it alone"
else
  ask LAZY_PELICAN_API_KEY "API key (the account key of the agent's user, starts with pacc_)" "" secret
  # umask so the key is never world-readable, even for an instant.
  (umask 077 && printf 'LAZY_PELICAN_API_KEY=%s\n' "$LAZY_PELICAN_API_KEY" >"$ENVFILE")
  chmod 600 "$ENVFILE"
  say "  key:     $ENVFILE written (mode 600)"
fi

install -d "$UNIT_DIR"
install -m 644 "$UNIT" "$UNIT_DIR/lazy-pelican-agent.service"
say "  service: $UNIT_DIR/lazy-pelican-agent.service"

# Check the setup before starting, with the same key the service will use. A failure is a warning, not an abort:
# the service still gets installed, and the message says what to fix.
say
say "Checking the setup (nothing is started on any game server):"
# shellcheck source=/dev/null
if (set -a; . "$ENVFILE"; set +a; "$BIN_DIR/lazy-pelican-agent" doctor -config "$CONFIG"); then
  :
else
  say
  say "Some checks failed (above). The service is installed; fix those, then restart it."
fi

if [ -n "${NO_START:-}" ] || [ -n "${LAZY_INSTALL_ROOTLESS:-}" ]; then
  say
  say "Not starting the service. To start it: systemctl daemon-reload && systemctl enable --now lazy-pelican-agent"
  exit 0
fi

systemctl daemon-reload
systemctl enable --now lazy-pelican-agent
# A restart picks up a new binary when this is an upgrade; on a first install it is a harmless no-op.
systemctl restart lazy-pelican-agent

say
say "Done. Useful commands:"
say "  systemctl status lazy-pelican-agent"
say "  journalctl -u lazy-pelican-agent -f"
