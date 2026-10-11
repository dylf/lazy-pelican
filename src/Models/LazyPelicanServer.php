<?php

namespace Dylf\LazyPelican\Models;

use App\Models\Server;
use Carbon\CarbonInterface;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

/**
 * Per-server Lazy Pelican settings: idle shutdown, and wake on connect.
 *
 * The table keeps its original name (idle_shutdown_settings) so existing installs don't need a table rename.
 *
 * @property int $id
 * @property int $server_id
 * @property bool $enabled
 * @property ?int $idle_minutes
 * @property ?int $grace_minutes
 * @property ?CarbonInterface $running_since
 * @property ?CarbonInterface $empty_since
 * @property ?CarbonInterface $last_stopped_at
 * @property bool $wake_enabled
 * @property ?array<int, int> $wake_ports
 * @property ?string $wake_match_prefix
 * @property ?int $wake_match_length
 * @property ?CarbonInterface $last_woken_at
 * @property Server $server
 */
class LazyPelicanServer extends Model
{
    protected $table = 'idle_shutdown_settings';

    protected $fillable = [
        'server_id',
        'enabled',
        'idle_minutes',
        'grace_minutes',
        'running_since',
        'empty_since',
        'last_stopped_at',
        'wake_enabled',
        'wake_ports',
        'wake_match_prefix',
        'wake_match_length',
        'last_woken_at',
    ];

    protected function casts(): array
    {
        return [
            'enabled' => 'bool',
            'wake_enabled' => 'bool',
            'wake_ports' => 'array',
            'running_since' => 'datetime',
            'empty_since' => 'datetime',
            'last_stopped_at' => 'datetime',
            'last_woken_at' => 'datetime',
        ];
    }

    public function server(): BelongsTo
    {
        return $this->belongsTo(Server::class);
    }

    public function effectiveIdleMinutes(): int
    {
        return max(1, $this->idle_minutes ?? (int) config('lazy-pelican.default_idle_minutes', 15));
    }

    public function effectiveGraceMinutes(): int
    {
        return max(0, $this->grace_minutes ?? (int) config('lazy-pelican.default_grace_minutes', 10));
    }

    /**
     * The game port(s) the agent should watch: the configured ones, or the server's primary allocation.
     *
     * @return array<int, int>
     */
    public function effectiveWakePorts(): array
    {
        $ports = collect($this->wake_ports ?? [])
            ->map(fn ($port) => (int) $port)
            ->filter(fn (int $port) => $port >= 1 && $port <= 65535)
            ->unique()
            ->values()
            ->all();

        if ($ports === [] && $this->server->allocation) {
            $ports = [(int) $this->server->allocation->port];
        }

        return $ports;
    }

    public function resetState(): void
    {
        $this->forceFill(['running_since' => null, 'empty_since' => null])->save();
    }
}
