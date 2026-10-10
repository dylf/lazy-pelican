<?php

namespace Dylf\LazyPelican\Models;

use App\Models\Server;
use Carbon\CarbonInterface;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

/**
 * @property int $id
 * @property int $server_id
 * @property bool $enabled
 * @property ?int $idle_minutes
 * @property ?int $grace_minutes
 * @property ?CarbonInterface $running_since
 * @property ?CarbonInterface $empty_since
 * @property ?CarbonInterface $last_stopped_at
 * @property Server $server
 */
class IdleShutdownSetting extends Model
{
    protected $fillable = [
        'server_id',
        'enabled',
        'idle_minutes',
        'grace_minutes',
        'running_since',
        'empty_since',
        'last_stopped_at',
    ];

    protected function casts(): array
    {
        return [
            'enabled' => 'bool',
            'running_since' => 'datetime',
            'empty_since' => 'datetime',
            'last_stopped_at' => 'datetime',
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

    public function resetState(): void
    {
        $this->forceFill(['running_since' => null, 'empty_since' => null])->save();
    }
}
