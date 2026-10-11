<?php

namespace Dylf\LazyPelican\Filament\Server\Pages;

use App\Enums\SubuserPermission;
use App\Facades\Activity;
use App\Filament\Server\Pages\ServerFormPage;
use App\Models\Server;
use Carbon\CarbonInterface;
use Dylf\LazyPelican\Models\LazyPelicanAgent;
use Dylf\LazyPelican\Models\LazyPelicanServer;
use Dylf\LazyPelican\Services\IdleShutdownService;
use Exception;
use Filament\Facades\Filament;
use Filament\Forms\Components\TagsInput;
use Filament\Forms\Components\TextInput;
use Filament\Forms\Components\Toggle;
use Filament\Notifications\Notification;
use Filament\Schemas\Components\Html;
use Filament\Schemas\Components\Section;
use Filament\Schemas\Components\Text;
use Filament\Schemas\Schema;
use Illuminate\Support\HtmlString;

class LazyPelicanPage extends ServerFormPage
{
    protected static string|\BackedEnum|null $navigationIcon = 'tabler-moon';

    protected static ?string $slug = 'lazy-pelican';

    protected static ?int $navigationSort = 31;

    public static function canAccess(): bool
    {
        return parent::canAccess()
            && user()?->can('lazy_pelican.update', Filament::getTenant());
    }

    protected function authorizeAccess(): void
    {
        abort_unless(user()?->can('lazy_pelican.update', $this->getRecord()), 403);
    }

    public static function getNavigationLabel(): string
    {
        return trans('lazy-pelican::messages.title');
    }

    public function getTitle(): string
    {
        return trans('lazy-pelican::messages.title');
    }

    protected function setting(): LazyPelicanServer
    {
        return LazyPelicanServer::firstOrNew(['server_id' => $this->getRecord()->id]);
    }

    protected function fillForm(): void
    {
        $setting = $this->setting();

        $this->form->fill([
            'enabled' => $setting->enabled,
            'idle_minutes' => $setting->idle_minutes,
            'grace_minutes' => $setting->grace_minutes,
            'wake_enabled' => $setting->wake_enabled,
            'wake_ports' => array_map('strval', $setting->wake_ports ?? []),
            'wake_match_prefix' => $setting->wake_match_prefix,
            'wake_match_length' => $setting->wake_match_length,
        ]);
    }

    public function form(Schema $schema): Schema
    {
        /** @var Server $server */
        $server = $this->getRecord();

        $hasPlayerCounter = IdleShutdownService::playerCounterAvailable();

        // @phpstan-ignore method.notFound
        $hasQuery = $hasPlayerCounter && $server->egg->gameQuery()->exists();

        return $schema
            ->statePath('data')
            ->components([
                Section::make(trans('lazy-pelican::messages.idle_section'))
                    ->description(trans('lazy-pelican::messages.idle_section_hint'))
                    ->schema([
                        Text::make(trans('lazy-pelican::messages.needs_player_counter'))
                            ->color('danger')
                            ->visible(!$hasPlayerCounter),

                        Text::make(trans('lazy-pelican::messages.needs_query'))
                            ->color('warning')
                            ->visible($hasPlayerCounter && !$hasQuery),

                        Toggle::make('enabled')
                            ->label(trans('lazy-pelican::messages.enabled'))
                            ->helperText(trans('lazy-pelican::messages.enabled_hint'))
                            ->disabled(!$hasPlayerCounter)
                            ->live()
                            ->afterStateUpdated(fn () => $this->save()),

                        TextInput::make('idle_minutes')
                            ->label(trans('lazy-pelican::messages.idle_minutes'))
                            ->numeric()
                            ->minValue(1)
                            ->maxValue(1440)
                            ->placeholder((string) config('lazy-pelican.default_idle_minutes'))
                            ->live(onBlur: true)
                            ->afterStateUpdated(fn () => $this->save()),

                        TextInput::make('grace_minutes')
                            ->label(trans('lazy-pelican::messages.grace_minutes'))
                            ->helperText(trans('lazy-pelican::messages.grace_hint'))
                            ->numeric()
                            ->minValue(0)
                            ->maxValue(120)
                            ->placeholder((string) config('lazy-pelican.default_grace_minutes'))
                            ->live(onBlur: true)
                            ->afterStateUpdated(fn () => $this->save()),
                    ]),

                Section::make(trans('lazy-pelican::messages.wake_section'))
                    ->description(trans('lazy-pelican::messages.wake_section_hint'))
                    ->schema([
                        Toggle::make('wake_enabled')
                            ->label(trans('lazy-pelican::messages.wake_enabled'))
                            ->live()
                            ->afterStateUpdated(fn () => $this->save()),

                        TagsInput::make('wake_ports')
                            ->label(trans('lazy-pelican::messages.wake_ports'))
                            ->helperText(trans('lazy-pelican::messages.wake_ports_hint'))
                            ->placeholder($server->allocation ? (string) $server->allocation->port : '')
                            ->nestedRecursiveRules(['integer', 'min:1', 'max:65535'])
                            ->live()
                            ->afterStateUpdated(fn () => $this->save()),

                        Section::make(trans('lazy-pelican::messages.wake_advanced'))
                            ->description(trans('lazy-pelican::messages.wake_advanced_hint'))
                            ->collapsed()
                            ->schema([
                                TextInput::make('wake_match_prefix')
                                    ->label(trans('lazy-pelican::messages.wake_prefix'))
                                    ->placeholder('2010000d')
                                    ->rule('regex:/^([0-9a-fA-F]{2})*$/')
                                    ->live(onBlur: true)
                                    ->afterStateUpdated(fn () => $this->save()),

                                TextInput::make('wake_match_length')
                                    ->label(trans('lazy-pelican::messages.wake_length'))
                                    ->numeric()
                                    ->minValue(1)
                                    ->maxValue(65535)
                                    ->live(onBlur: true)
                                    ->afterStateUpdated(fn () => $this->save()),
                            ]),
                    ]),

                Section::make(trans('lazy-pelican::messages.status_section'))
                    // A closure, so the lines are rebuilt on every render: turning wake on or saving a change shows up at once.
                    ->schema([Html::make(fn () => $this->statusHtml($server))]),
            ]);
    }

    protected function statusHtml(Server $server): HtmlString
    {
        $colors = ['success' => '#22c55e', 'warning' => '#f59e0b', 'danger' => '#ef4444', 'gray' => 'inherit'];

        $html = '';
        foreach ($this->statusLines($server) as $line) {
            $html .= '<p style="margin: 0 0 .5rem; font-size: .875rem; color: ' . $colors[$line['color']] . ';">' . e($line['text']) . '</p>';
        }

        return new HtmlString($html);
    }

    /**
     * What an admin needs to know about this server's wake setup, in plain sentences.
     *
     * @return array<int, array{text: string, color: string}>
     */
    protected function statusLines(Server $server): array
    {
        $row = $this->setting();
        $never = trans('lazy-pelican::messages.never');
        $when = fn (?CarbonInterface $at) => $at ? $at->diffForHumans() : $never;

        $lines = [
            ['text' => trans('lazy-pelican::messages.last_woken', ['when' => $when($row->last_woken_at)]), 'color' => 'gray'],
            ['text' => trans('lazy-pelican::messages.last_stopped', ['when' => $when($row->last_stopped_at)]), 'color' => 'gray'],
        ];

        if (!$row->wake_enabled) {
            return $lines;
        }

        $agents = LazyPelicanAgent::visibleForNode($server->node);

        if ($agents->isEmpty()) {
            $lines[] = ['text' => trans('lazy-pelican::messages.agent_none'), 'color' => 'warning'];

            return $lines;
        }

        foreach ($agents as $agent) {
            $state = $agent->state();
            $vars = ['host' => $agent->hostname, 'version' => $agent->version, 'node' => $server->node->name, 'when' => $agent->last_seen_at->diffForHumans(), 'state' => $agent->stateLabel()];

            $lines[] = ['text' => trans('lazy-pelican::messages.agent_line', $vars), 'color' => $state === LazyPelicanAgent::STATE_CONNECTED ? 'success' : 'warning'];

            if (!$agent->capture_ok) {
                $lines[] = ['text' => trans('lazy-pelican::messages.agent_capture_failed', ['host' => $agent->hostname, 'error' => $agent->capture_error ?: '?']), 'color' => 'danger'];
            }

            if ($agent->user && !$agent->user->can(SubuserPermission::ControlStart, $server)) {
                $lines[] = ['text' => trans('lazy-pelican::messages.agent_cannot_start', ['user' => $agent->user->username]), 'color' => 'danger'];
            }

            $watching = collect($agent->watching ?? [])->pluck('identifier')->all();
            if ($state === LazyPelicanAgent::STATE_CONNECTED && $agent->capture_ok && !in_array($server->uuid_short, $watching, true)) {
                $lines[] = ['text' => trans('lazy-pelican::messages.agent_not_watching', ['host' => $agent->hostname]), 'color' => 'warning'];
            }
        }

        return $lines;
    }

    public function save(): void
    {
        abort_unless(user()?->can('lazy_pelican.update', $this->getRecord()), 403);

        try {
            $data = $this->form->getState();

            $ports = array_values(array_unique(array_filter(
                array_map('intval', $data['wake_ports'] ?? []),
                fn (int $port) => $port >= 1 && $port <= 65535,
            )));

            $setting = $this->setting();

            $idle = [
                'enabled' => (bool) ($data['enabled'] ?? false),
                'idle_minutes' => filled($data['idle_minutes'] ?? null) ? (int) $data['idle_minutes'] : null,
                'grace_minutes' => filled($data['grace_minutes'] ?? null) ? (int) $data['grace_minutes'] : null,
            ];

            // Changing the idle settings restarts the idle clock. Changing wake settings must not.
            $idleChanged = !$setting->exists;
            foreach ($idle as $key => $value) {
                $idleChanged = $idleChanged || $setting->{$key} !== $value;
            }

            $setting->fill($idle + [
                'wake_enabled' => (bool) ($data['wake_enabled'] ?? false),
                'wake_ports' => $ports ?: null,
                'wake_match_prefix' => filled($data['wake_match_prefix'] ?? null) ? strtolower((string) $data['wake_match_prefix']) : null,
                'wake_match_length' => filled($data['wake_match_length'] ?? null) ? (int) $data['wake_match_length'] : null,
            ]);
            $setting->save();

            if ($idleChanged) {
                $setting->resetState();
            }

            Activity::event('server:lazy-pelican.update')
                ->property('idle_shutdown', $setting->enabled)
                ->property('wake_on_connect', $setting->wake_enabled)
                ->log();

            Notification::make()
                ->title(trans('lazy-pelican::messages.saved'))
                ->success()
                ->send();
        } catch (Exception $exception) {
            report($exception);

            Notification::make()
                ->title(trans('lazy-pelican::messages.save_failed'))
                ->body($exception->getMessage())
                ->danger()
                ->send();
        }
    }
}
