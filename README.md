# Lazy Pelican (Pelican plugin)

Stops a game server automatically after it has had **no players** for a set time.
Starting is manual: players you've added as subusers can press Start in the panel.

## How it works
- Every minute a scheduled command (`lazy-pelican:check`) looks at each server that has idle shutdown enabled.
- Player counts come from the **Player Counter** plugin's Game Query for the server's egg
  (A2S/Source covers Valheim on the game port + 1, e.g. 2456 -> query 2457).
- Not running -> nothing happens. Running -> a startup grace period, then idle time is counted.
- When the player count has been 0 for the idle time, a normal panel `stop` power action is sent
  (Valheim's egg stops with ^C, so the world is saved).
- Fail-safe: if the player count can't be read, the idle timer resets and the server is **not** stopped.

## Requirements
- Pelican panel with the plugin system
- `player-counter` plugin installed, with a Game Query linked to the egg
- The panel's scheduler and queue worker running (`php artisan schedule:run` via cron)

## Setup (Valheim)
1. Install and enable `player-counter`. Admin -> Game Queries -> create one: type **Source**,
   query port offset **1**, link it to the Valheim egg.
2. Install this plugin (copy folder `lazy-pelican` to `plugins/`, `php artisan p:plugin:install`).
3. Open the server -> **Idle Shutdown**, enable it, set minutes.
4. Give subusers the **Idle Shutdown** permission if they should manage it.

## Config (env)
| Variable | Default |
|---|---|
| `LAZY_PELICAN_DEFAULT_IDLE_MINUTES` | 15 |
| `LAZY_PELICAN_DEFAULT_GRACE_MINUTES` | 10 |
| `LAZY_PELICAN_VERBOSE` | false |

Test by hand: `php artisan lazy-pelican:check -v`

## Roadmap
- Wake-on-connect via a node-side agent (separate component).

## Releasing

Releases are automated with [release-please](https://github.com/googleapis/release-please). Write commit messages in
the conventional style (`feat: ...`, `fix: ...`, `feat!: ...` for breaking changes). On every push to `main` a
"release" pull request is kept up to date with the next version (bumped in `plugin.json`) and a changelog.

**To release, merge that pull request.** That creates the tag and GitHub Release, and the workflow then attaches
`lazy-pelican.zip` (built with `git archive`; dev files are left out via `.gitattributes`) and an `update.json` with
the version, download URL and SHA-256. The panel reads that file through `update_url` in `plugin.json`, so installed
copies get the one-click Update button.

CI (Pint, PHPStan, tests) runs on every push to `main` before release-please does, so a failing build can't produce a
release. If it fails after a release PR is merged, fix it on `main`; the next successful run creates the release.

One-time repo setting: Settings > Actions > General > "Allow GitHub Actions to create and approve pull requests".

## License

MIT, see [LICENSE](LICENSE).

## Development

CI follows the official `pelican/plugins` setup (Pint and PHPStan against a checkout of the panel), plus a small PHPUnit suite.

```bash
composer install
vendor/bin/pint --test     # code style
vendor/bin/phpunit         # structure checks (no panel needed)
```

PHPStan only runs in CI, because it analyses against the panel's classes.
