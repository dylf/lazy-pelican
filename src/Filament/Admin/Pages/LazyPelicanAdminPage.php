<?php

namespace Dylf\LazyPelican\Filament\Admin\Pages;

use App\Enums\SubuserPermission;
use App\Models\Node;
use Dylf\LazyPelican\Models\LazyPelicanAgent;
use Dylf\LazyPelican\Models\LazyPelicanServer;
use Dylf\LazyPelican\Services\IdleShutdownService;
use Dylf\LazyPelican\Support\WakeServerRow;
use Filament\Notifications\Notification;
use Filament\Pages\Page;
use Illuminate\Support\Collection;

class LazyPelicanAdminPage extends Page
{
    protected static string|\BackedEnum|null $navigationIcon = 'tabler-moon';

    protected static ?string $slug = 'lazy-pelican';

    protected string $view = 'lazy-pelican::admin';

    public static function canAccess(): bool
    {
        return user()?->isRootAdmin() ?? false;
    }

    public static function getNavigationLabel(): string
    {
        return trans('lazy-pelican::messages.admin_title');
    }

    public function getTitle(): string
    {
        return trans('lazy-pelican::messages.admin_title');
    }

    /** Forget one agent. A live agent reappears on its next heartbeat, so only ones that aren't connected are removable. */
    public function removeAgent(int $id): void
    {
        abort_unless(static::canAccess(), 403);

        $agent = LazyPelicanAgent::query()->find($id);

        if ($agent && !$agent->isConnected()) {
            $agent->delete();

            Notification::make()
                ->title(trans('lazy-pelican::messages.admin_agent_removed'))
                ->success()
                ->send();
        }
    }

    public function agentRetentionDays(): int
    {
        return max(1, (int) config('lazy-pelican.agent_retention_days'));
    }

    public function playerCounterAvailable(): bool
    {
        return IdleShutdownService::playerCounterAvailable();
    }

    /** @return Collection<int, LazyPelicanAgent> */
    public function agents(): Collection
    {
        return LazyPelicanAgent::query()->with('user')->orderByDesc('last_seen_at')->get();
    }

    /**
     * Servers with wake on connect enabled, and whether anything is actually going to wake them.
     *
     * @return Collection<int, WakeServerRow>
     */
    public function wakeServers(): Collection
    {
        $agents = LazyPelicanAgent::query()->with('user')->get();

        return LazyPelicanServer::query()
            ->where('wake_enabled', true)
            ->with(['server.node', 'server.allocation'])
            ->get()
            ->map(function (LazyPelicanServer $row) use ($agents) {
                $server = $row->server;
                $ports = $row->effectiveWakePorts();
                $covering = $agents->filter(fn (LazyPelicanAgent $agent) => $agent->coversNode($server->node));

                $problem = match (true) {
                    $covering->isEmpty() => 'No agent has reported in for this node',
                    $covering->doesntContain(fn (LazyPelicanAgent $agent) => $agent->isConnected()) => 'The agent for this node is not responding',
                    $covering->doesntContain(fn (LazyPelicanAgent $agent) => $agent->isConnected() && $agent->capture_ok) => 'The agent is running but cannot capture packets',
                    $covering->doesntContain(fn (LazyPelicanAgent $agent) => $agent->user?->can(SubuserPermission::ControlStart, $server)) => 'The agent\'s user cannot start this server',
                    default => null,
                };

                return new WakeServerRow(
                    name: $server->name,
                    node: $server->node->name,
                    ports: $ports === [] ? '-' : implode(', ', $ports),
                    lastWoken: $row->last_woken_at?->diffForHumans() ?? $this->never(),
                    problem: $problem,
                );
            })
            ->values();
    }

    private function never(): string
    {
        $text = trans('lazy-pelican::messages.never');

        return is_string($text) ? $text : 'never';
    }

    public function panelUrl(): string
    {
        return rtrim((string) config('app.url'), '/');
    }

    public function firstNodeName(): string
    {
        return Node::query()->orderBy('id')->value('name') ?? 'my-node';
    }

    /** @return Collection<int, Node> */
    public function nodes(): Collection
    {
        return Node::query()->orderBy('id')->get();
    }

    public function configSnippet(): string
    {
        return <<<YAML
        # /etc/lazy-pelican-agent/config.yml
        # Everything else (which servers, ports, thresholds) comes from this panel.
        panel_url: {$this->panelUrl()}
        node: {$this->firstNodeName()}
        YAML;
    }

    public function composeSnippet(): string
    {
        return <<<'YAML'
        services:
          lazy-pelican-agent:
            image: ghcr.io/dylf/lazy-pelican-agent:latest
            restart: always
            network_mode: host
            cap_drop: [ALL]
            cap_add: [NET_RAW]
            read_only: true
            security_opt: ["no-new-privileges:true"]
            environment:
              LAZY_PELICAN_API_KEY: "pacc_..."
            volumes:
              - ./config.yml:/etc/lazy-pelican-agent/config.yml:ro
        YAML;
    }

    public function systemdSnippet(): string
    {
        return <<<'SH'
        sudo install -m 755 lazy-pelican-agent /usr/local/bin/lazy-pelican-agent
        sudo install -d /etc/lazy-pelican-agent            # put config.yml (above) in here
        echo 'LAZY_PELICAN_API_KEY=pacc_...' | sudo install -m 600 /dev/stdin /etc/lazy-pelican-agent/env
        sudo cp lazy-pelican-agent.service /etc/systemd/system/
        sudo systemctl enable --now lazy-pelican-agent
        lazy-pelican-agent doctor -config /etc/lazy-pelican-agent/config.yml
        SH;
    }
}
