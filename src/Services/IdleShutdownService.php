<?php

namespace Dylf\LazyPelican\Services;

use App\Enums\ContainerStatus;
use App\Models\Server;
use App\Repositories\Daemon\DaemonServerRepository;
use Boy132\PlayerCounter\Extensions\Query\QueryTypeService;
use Boy132\PlayerCounter\Models\GameQuery;
use Dylf\LazyPelican\Models\LazyPelicanServer;
use Exception;
use Illuminate\Support\Facades\Log;

class IdleShutdownService
{
    public function __construct(private readonly DaemonServerRepository $daemonServerRepository) {}

    /**
     * Run one check for every enabled server.
     *
     * @return array<int, string> server id => decision, for logging/tests
     */
    public function checkAll(): array
    {
        $results = [];

        if (!static::playerCounterAvailable()) {
            $count = LazyPelicanServer::query()->where('enabled', true)->count();

            if ($count > 0) {
                Log::warning(trans('lazy-pelican::messages.player_counter_missing_log', ['count' => $count]));
            }

            return $results;
        }

        LazyPelicanServer::query()
            ->where('enabled', true)
            ->with('server')
            ->each(function (LazyPelicanServer $setting) use (&$results) {
                try {
                    $results[$setting->server_id] = $this->check($setting);
                } catch (Exception $exception) {
                    // One broken server must never stop the others from being checked.
                    report($exception);
                    $results[$setting->server_id] = 'error: ' . $exception->getMessage();
                }
            });

        return $results;
    }

    /**
     * Decide what to do with a single server. Fails safe: anything uncertain resets
     * the idle timer and never stops the server.
     */
    public function check(LazyPelicanServer $setting): string
    {
        $server = $setting->server;

        if ($server->isSuspended() || $server->isInConflictState()) {
            $setting->resetState();

            return 'skipped: suspended or in conflict state';
        }

        if ($server->retrieveStatus() !== ContainerStatus::Running) {
            $setting->resetState();

            return 'skipped: not running';
        }

        $now = now();

        // First time we see it running: start the grace period.
        if (!$setting->running_since) {
            $setting->forceFill(['running_since' => $now, 'empty_since' => null])->save();
        }

        $graceEnds = $setting->running_since->copy()->addMinutes($setting->effectiveGraceMinutes());

        if ($now->lt($graceEnds)) {
            $setting->forceFill(['empty_since' => null])->save();

            return 'grace period until ' . $graceEnds->toDateTimeString();
        }

        $players = $this->countPlayers($server);

        if ($players === null) {
            // Query failed or isn't configured. Do not guess.
            $setting->forceFill(['empty_since' => null])->save();

            return 'unknown player count (query failed), not stopping';
        }

        if ($players > 0) {
            $setting->forceFill(['empty_since' => null])->save();

            return "$players player(s) online";
        }

        if (!$setting->empty_since) {
            $setting->forceFill(['empty_since' => $now])->save();
        }

        $emptyFor = (int) $setting->empty_since->diffInMinutes($now, true);

        if ($emptyFor < $setting->effectiveIdleMinutes()) {
            return "empty for $emptyFor/{$setting->effectiveIdleMinutes()} min";
        }

        return $this->stop($setting, $server, $emptyFor);
    }

    /**
     * True only when player-counter is installed AND enabled.
     *
     * class_exists() alone is not enough: the panel registers every plugin's PSR-4 namespace even when the
     * plugin is disabled, so its classes autoload but its service provider (and the Egg::gameQuery relation)
     * never run. The QueryTypeService singleton is only bound by player-counter's provider.
     */
    public static function playerCounterAvailable(): bool
    {
        return class_exists(GameQuery::class) && app()->bound(QueryTypeService::class);
    }

    /**
     * @return ?int null when the count can't be determined
     */
    protected function countPlayers(Server $server): ?int
    {
        // Soft dependency on the player-counter plugin: it owns the game query layer.
        if (!static::playerCounterAvailable() || !GameQuery::canRunQuery($server->allocation)) {
            return null;
        }

        /** @var ?GameQuery $gameQuery */
        $gameQuery = $server->egg->gameQuery; // @phpstan-ignore property.notFound

        if (!$gameQuery) {
            return null;
        }

        $data = $gameQuery->runQuery($server);

        if (!$data) {
            return null;
        }

        return (int) $data['current_players'];
    }

    protected function stop(LazyPelicanServer $setting, Server $server, int $emptyFor): string
    {
        $this->daemonServerRepository->setServer($server)->power('stop')->throw();

        $setting->forceFill([
            'running_since' => null,
            'empty_since' => null,
            'last_stopped_at' => now(),
        ])->save();

        // Drop the cached status so the panel UI reflects the stop quickly.
        cache()->forget("servers.$server->uuid.status");

        Log::info("lazy-pelican: stopped server {$server->uuid} ({$server->name}) after $emptyFor minutes with no players");

        return "stopped after $emptyFor min idle";
    }
}
