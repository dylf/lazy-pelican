<?php

namespace Dylf\LazyPelican\Models;

use App\Models\Node;
use App\Models\User;
use Carbon\CarbonInterface;
use Dylf\LazyPelican\Support\NodeMatcher;
use Illuminate\Database\Eloquent\Collection;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

/**
 * A lazy-pelican-agent that has reported in (its heartbeat).
 *
 * @property int $id
 * @property string $agent_key
 * @property int $user_id
 * @property ?string $node_key
 * @property string $hostname
 * @property string $version
 * @property ?string $interface
 * @property bool $capture_ok
 * @property ?string $capture_error
 * @property ?array<int, array{identifier: string, last_detection_at: ?string, last_wake_at: ?string}> $watching
 * @property CarbonInterface $last_seen_at
 * @property ?User $user
 */
class LazyPelicanAgent extends Model
{
    public const STATE_CONNECTED = 'connected';

    public const STATE_STALE = 'stale';

    public const STATE_OFFLINE = 'offline';

    /** Heartbeats arrive every 30s; this is how long we wait before calling the agent gone. */
    public const CONNECTED_SECONDS = 90;

    public const STALE_SECONDS = 600;

    protected $table = 'lazy_pelican_agents';

    protected $fillable = [
        'agent_key',
        'user_id',
        'node_key',
        'hostname',
        'version',
        'interface',
        'capture_ok',
        'capture_error',
        'watching',
        'last_seen_at',
    ];

    protected function casts(): array
    {
        return [
            'capture_ok' => 'bool',
            'watching' => 'array',
            'last_seen_at' => 'datetime',
        ];
    }

    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }

    public static function keyFor(int $userId, ?string $node, string $hostname): string
    {
        return sha1($userId . '|' . strtolower(trim((string) $node)) . '|' . strtolower($hostname));
    }

    public function state(?CarbonInterface $now = null): string
    {
        $seconds = ($now ?? now())->getTimestamp() - $this->last_seen_at->getTimestamp();

        return match (true) {
            $seconds <= self::CONNECTED_SECONDS => self::STATE_CONNECTED,
            $seconds <= self::STALE_SECONDS => self::STATE_STALE,
            default => self::STATE_OFFLINE,
        };
    }

    public function stateLabel(?CarbonInterface $now = null): string
    {
        return match ($this->state($now)) {
            self::STATE_CONNECTED => trans('lazy-pelican::messages.state_connected'),
            self::STATE_STALE => trans('lazy-pelican::messages.state_stale'),
            default => trans('lazy-pelican::messages.state_offline'),
        };
    }

    public function isConnected(): bool
    {
        return $this->state() === self::STATE_CONNECTED;
    }

    /** An agent that didn't name a node watches every node it can see. */
    public function coversNode(Node $node): bool
    {
        return NodeMatcher::matches($this->node_key, $node->id, $node->name, $node->fqdn);
    }

    /**
     * Forget agents that haven't reported in for $days days. Returns how many were removed.
     */
    public static function prune(int $days): int
    {
        return static::query()->where('last_seen_at', '<', now()->subDays(max(1, $days)))->delete();
    }

    /**
     * The agents worth showing for a node: any that are connected or recently seen. If every agent for the node is
     * long gone, just the most recent one, so "the agent isn't responding" still shows up instead of nothing.
     *
     * @return Collection<int, static>
     */
    public static function visibleForNode(Node $node): Collection
    {
        $agents = static::forNode($node);
        $alive = $agents->filter(fn (self $agent) => $agent->state() !== self::STATE_OFFLINE)->values();

        return $alive->isNotEmpty() ? $alive : $agents->take(1)->values();
    }

    /** @return Collection<int, static> */
    public static function forNode(Node $node): Collection
    {
        return static::query()->with('user')->orderByDesc('last_seen_at')->get()->filter(fn (self $agent) => $agent->coversNode($node))->values();
    }
}
