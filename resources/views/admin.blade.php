<x-filament-panels::page>
    @php
        $agents = $this->agents();
        $servers = $this->wakeServers();
        $cell = 'padding: .5rem .75rem; text-align: left; border-bottom: 1px solid rgba(127,127,127,.25);';
        $pre = 'overflow-x: auto; padding: .75rem 1rem; border-radius: .5rem; background: rgba(127,127,127,.12); font-size: .8rem; line-height: 1.4;';
    @endphp

    @unless ($this->playerCounterAvailable())
        <x-filament::section>
            <p style="color: rgb(var(--danger-500, 239 68 68));">{{ trans('lazy-pelican::messages.needs_player_counter_admin') }}</p>
        </x-filament::section>
    @endunless

    <x-filament::section :heading="trans('lazy-pelican::messages.admin_agents')">
        @if ($agents->isEmpty())
            <p>{{ trans('lazy-pelican::messages.admin_agents_empty') }}</p>
        @else
            <table style="width: 100%; border-collapse: collapse; font-size: .875rem;">
                <thead>
                    <tr>
                        <th style="{{ $cell }}">Host</th>
                        <th style="{{ $cell }}">Node</th>
                        <th style="{{ $cell }}">Status</th>
                        <th style="{{ $cell }}">Packet capture</th>
                        <th style="{{ $cell }}">Version</th>
                        <th style="{{ $cell }}">API key user</th>
                        <th style="{{ $cell }}">Last heard</th>
                        <th style="{{ $cell }}"></th>
                    </tr>
                </thead>
                <tbody>
                    @foreach ($agents as $agent)
                        @php $state = $agent->state(); @endphp
                        <tr>
                            <td style="{{ $cell }}">{{ $agent->hostname }}</td>
                            <td style="{{ $cell }}">{{ $agent->node_key ?: 'any' }}</td>
                            <td style="{{ $cell }}">
                                <x-filament::badge :color="$state === 'connected' ? 'success' : ($state === 'stale' ? 'warning' : 'danger')">
                                    {{ $agent->stateLabel() }}
                                </x-filament::badge>
                            </td>
                            <td style="{{ $cell }}">
                                @if ($agent->capture_ok)
                                    <x-filament::badge color="success">working</x-filament::badge>
                                @else
                                    <x-filament::badge color="danger">failed</x-filament::badge>
                                    <div style="font-size: .75rem; margin-top: .25rem;">{{ $agent->capture_error }}</div>
                                @endif
                            </td>
                            <td style="{{ $cell }}">{{ $agent->version }}</td>
                            <td style="{{ $cell }}">{{ $agent->user?->username ?? '?' }}</td>
                            <td style="{{ $cell }}">{{ $agent->last_seen_at->diffForHumans() }}</td>
                            <td style="{{ $cell }} text-align: right;">
                                @if ($state !== 'connected')
                                    <x-filament::button size="xs" color="danger" outlined
                                        wire:click="removeAgent({{ $agent->id }})"
                                        wire:confirm="{{ trans('lazy-pelican::messages.admin_agent_remove_confirm') }}">
                                        {{ trans('lazy-pelican::messages.admin_agent_remove') }}
                                    </x-filament::button>
                                @endif
                            </td>
                        </tr>
                    @endforeach
                </tbody>
            </table>
            <p style="font-size: .8rem; margin-top: .75rem; opacity: .75;">
                {{ trans('lazy-pelican::messages.admin_agents_note', ['days' => $this->agentRetentionDays()]) }}
            </p>
        @endif
    </x-filament::section>

    <x-filament::section :heading="trans('lazy-pelican::messages.admin_servers')">
        @if ($servers->isEmpty())
            <p>{{ trans('lazy-pelican::messages.admin_servers_empty') }}</p>
        @else
            <table style="width: 100%; border-collapse: collapse; font-size: .875rem;">
                <thead>
                    <tr>
                        <th style="{{ $cell }}">Server</th>
                        <th style="{{ $cell }}">Node</th>
                        <th style="{{ $cell }}">Watched ports</th>
                        <th style="{{ $cell }}">Last woken</th>
                        <th style="{{ $cell }}">Will it wake?</th>
                    </tr>
                </thead>
                <tbody>
                    @foreach ($servers as $server)
                        <tr>
                            <td style="{{ $cell }}">{{ $server->name }}</td>
                            <td style="{{ $cell }}">{{ $server->node }}</td>
                            <td style="{{ $cell }}">{{ $server->ports }}</td>
                            <td style="{{ $cell }}">{{ $server->lastWoken }}</td>
                            <td style="{{ $cell }}">
                                @if ($server->problem)
                                    <x-filament::badge color="danger">{{ $server->problem }}</x-filament::badge>
                                @else
                                    <x-filament::badge color="success">yes</x-filament::badge>
                                @endif
                            </td>
                        </tr>
                    @endforeach
                </tbody>
            </table>
        @endif
        <p style="font-size: .8rem; margin-top: .75rem; opacity: .75;">
            "Will it wake?" checks that an agent is connected for the node, can capture packets, and its user may start the
            server. It can't see your router: the game port still has to be forwarded to the Wings host.
        </p>
    </x-filament::section>

    <x-filament::section :heading="trans('lazy-pelican::messages.admin_setup')" collapsible>
        <ol style="list-style: decimal; padding-left: 1.25rem; display: grid; gap: .75rem;">
            <li>
                <strong>Create a user for the agent</strong> (Admin > Users). It must be a normal user, <em>not</em> an admin.
                Add it as a subuser on every server you want woken, with only the <strong>Control > Start</strong> permission.
            </li>
            <li>
                <strong>Create a client API key as that user:</strong> log in as it, then Account > API Keys. It starts with
                <code>pacc_</code>. (The admin "Application" key under Admin > API Keys will not work.) The key is shown once.
            </li>
            <li>
                <strong>Get the agent</strong> on the Wings host: download the tarball for your architecture from the
                <a href="https://github.com/dylf/lazy-pelican/releases" target="_blank" rel="noopener" style="text-decoration: underline;">releases page</a>,
                or use the container image. Run it on the Docker host next to Wings, not inside the Wings container.
            </li>
            <li>
                <strong>Config file.</strong> This is all it needs; servers and ports come from this panel. Your nodes:
                @foreach ($this->nodes() as $node)
                    <code>{{ $node->name }}</code>@unless ($loop->last), @endunless
                @endforeach
                <pre style="{{ $pre }}">{{ $this->configSnippet() }}</pre>
            </li>
            <li>
                <strong>Run it,</strong> either in Docker:
                <pre style="{{ $pre }}">{{ $this->composeSnippet() }}</pre>
                or on bare metal with systemd:
                <pre style="{{ $pre }}">{{ $this->systemdSnippet() }}</pre>
            </li>
            <li>
                <strong>Turn on "Wake on connect"</strong> for each server on its Lazy Pelican page. The agent shows up in the
                table above within about 30 seconds. Forward each game port (UDP) from your router to the Wings host.
            </li>
        </ol>
        <p style="font-size: .8rem; margin-top: .75rem; opacity: .75;">
            Detection thresholds, cooldown and the hourly cap are set in this plugin's settings (Admin > Plugins).
            The agent refreshes them every minute.
        </p>
    </x-filament::section>
</x-filament-panels::page>
