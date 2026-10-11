<?php

namespace Dylf\LazyPelican\Http\Controllers\Api\Client;

use App\Enums\SubuserPermission;
use App\Facades\Activity;
use App\Http\Controllers\Api\Client\ClientApiController;
use App\Models\Server;
use App\Models\User;
use Dedoc\Scramble\Attributes\Group;
use Dylf\LazyPelican\Models\LazyPelicanAgent;
use Dylf\LazyPelican\Models\LazyPelicanServer;
use Dylf\LazyPelican\Support\NodeMatcher;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Http\Response;

/**
 * What the lazy-pelican-agent talks to. It authenticates with an ordinary client API key, and only ever sees servers that
 * key's user is allowed to start.
 */
#[Group('Lazy Pelican Agent')]
class AgentController extends ClientApiController
{
    public const API_VERSION = 1;

    /**
     * Agent config
     *
     * The servers the agent should watch (wake on connect enabled, and the key's user may start them), plus the
     * detector and limit settings. Pass `node` (id, name or FQDN) to only get that node's servers.
     */
    public function config(Request $request): JsonResponse
    {
        /** @var User $user */
        $user = $request->user();
        $node = $request->query('node');
        $node = is_string($node) ? trim($node) : '';

        $servers = LazyPelicanServer::query()
            ->where('wake_enabled', true)
            ->with(['server.node', 'server.allocation'])
            ->get()
            ->filter(fn (LazyPelicanServer $row) => $user->can(SubuserPermission::ControlStart, $row->server))
            ->filter(fn (LazyPelicanServer $row) => NodeMatcher::matches($node, $row->server->node->id, $row->server->node->name, $row->server->node->fqdn))
            ->map(fn (LazyPelicanServer $row) => [
                'identifier' => $row->server->uuid_short,
                'name' => $row->server->name,
                'node' => $row->server->node->name,
                'ports' => $row->effectiveWakePorts(),
                'match' => [
                    'prefix' => (string) $row->wake_match_prefix,
                    'length' => (int) $row->wake_match_length,
                ],
            ])
            ->filter(fn (array $server) => $server['ports'] !== [])
            ->values();

        return response()->json([
            'api_version' => self::API_VERSION,
            'detector' => [
                'packets' => max(1, (int) config('lazy-pelican.detector_packets')),
                'window_seconds' => max(1, (int) config('lazy-pelican.detector_window')),
                'holdoff_seconds' => max(0, (int) config('lazy-pelican.detector_holdoff')),
            ],
            'limits' => [
                'cooldown_seconds' => max(0, (int) config('lazy-pelican.wake_cooldown')) * 60,
                'max_wakes_per_hour' => max(1, (int) config('lazy-pelican.wake_max_per_hour')),
            ],
            'servers' => $servers,
        ]);
    }

    /**
     * Agent heartbeat
     *
     * Lets the panel show whether an agent is alive, which version it runs, and whether packet capture works.
     */
    public function heartbeat(Request $request): Response
    {
        /** @var User $user */
        $user = $request->user();

        $data = $request->validate([
            'agent_version' => ['required', 'string', 'max:64'],
            'hostname' => ['required', 'string', 'max:255'],
            'node' => ['nullable', 'string', 'max:255'],
            'interface' => ['nullable', 'string', 'max:64'],
            'capture' => ['required', 'array'],
            'capture.ok' => ['required', 'boolean'],
            'capture.error' => ['nullable', 'string', 'max:1000'],
            'servers' => ['nullable', 'array', 'max:200'],
            'servers.*.identifier' => ['required', 'string', 'max:32'],
            'servers.*.last_detection_at' => ['nullable', 'date'],
            'servers.*.last_wake_at' => ['nullable', 'date'],
        ]);

        $node = filled($data['node'] ?? null) ? $data['node'] : null;

        LazyPelicanAgent::query()->updateOrCreate(
            ['agent_key' => LazyPelicanAgent::keyFor($user->id, $node, $data['hostname'])],
            [
                'user_id' => $user->id,
                'node_key' => $node,
                'hostname' => $data['hostname'],
                'version' => $data['agent_version'],
                'interface' => $data['interface'] ?? null,
                'capture_ok' => (bool) $data['capture']['ok'],
                'capture_error' => $data['capture']['ok'] ? null : ($data['capture']['error'] ?? null),
                'watching' => array_values($data['servers'] ?? []),
                'last_seen_at' => now(),
            ],
        );

        return response()->noContent();
    }

    /**
     * Agent wake
     *
     * Records that the agent started a server because a player tried to join, in the server's activity log.
     */
    public function wake(Request $request): Response
    {
        /** @var User $user */
        $user = $request->user();

        $data = $request->validate([
            'identifier' => ['required', 'string', 'max:32'],
            'at' => ['nullable', 'date'],
        ]);

        /** @var ?Server $server */
        $server = Server::query()->where('uuid_short', $data['identifier'])->first();
        $row = $server ? LazyPelicanServer::query()->where('server_id', $server->id)->where('wake_enabled', true)->first() : null;

        abort_unless($server && $row, Response::HTTP_NOT_FOUND, 'No such server with wake on connect enabled');
        abort_unless($user->can(SubuserPermission::ControlStart, $server), Response::HTTP_FORBIDDEN);

        $row->forceFill(['last_woken_at' => now()])->save();

        Activity::event('server:lazy-pelican.wake')
            ->subject($server)
            ->property('source', 'lazy-pelican-agent')
            ->log();

        return response()->noContent();
    }
}
