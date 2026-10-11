<?php

namespace Dylf\LazyPelican;

use App\Contracts\Plugins\HasPluginSettings;
use App\Traits\EnvironmentWriterTrait;
use Dylf\LazyPelican\Services\IdleShutdownService;
use Filament\Contracts\Plugin;
use Filament\Forms\Components\TextInput;
use Filament\Notifications\Notification;
use Filament\Panel;
use Filament\Schemas\Components\Text;

class LazyPelicanPlugin implements HasPluginSettings, Plugin
{
    use EnvironmentWriterTrait;

    public function getId(): string
    {
        return 'lazy-pelican';
    }

    public function register(Panel $panel): void
    {
        $id = str($panel->getId())->title();

        $panel->discoverPages(plugin_path($this->getId(), "src/Filament/$id/Pages"), "Dylf\\LazyPelican\\Filament\\$id\\Pages");
    }

    public function boot(Panel $panel): void {}

    public function getSettingsFormData(): array
    {
        return config('lazy-pelican');
    }

    public function getSettingsForm(): array
    {
        $available = IdleShutdownService::playerCounterAvailable();

        $number = fn (string $key, int $min, $label, $help) => TextInput::make($key)
            ->label($label)
            ->helperText($help)
            ->numeric()
            ->minValue($min)
            ->required()
            ->default(fn () => config('lazy-pelican.' . $key));

        return [
            Text::make(trans($available ? 'lazy-pelican::messages.player_counter_ok_admin' : 'lazy-pelican::messages.needs_player_counter_admin'))
                ->color($available ? 'success' : 'danger'),
            $number('default_idle_minutes', 1, trans('lazy-pelican::messages.default_idle_minutes'), trans('lazy-pelican::messages.default_idle_minutes_help')),
            $number('default_grace_minutes', 0, trans('lazy-pelican::messages.default_grace_minutes'), trans('lazy-pelican::messages.default_grace_minutes_help')),
            Text::make(trans('lazy-pelican::messages.agent_refresh_note')),
            $number('detector_packets', 1, trans('lazy-pelican::messages.detector_packets'), trans('lazy-pelican::messages.detector_packets_help')),
            $number('detector_window', 1, trans('lazy-pelican::messages.detector_window'), trans('lazy-pelican::messages.detector_window_help')),
            $number('detector_holdoff', 0, trans('lazy-pelican::messages.detector_holdoff'), trans('lazy-pelican::messages.detector_holdoff_help')),
            $number('wake_cooldown', 0, trans('lazy-pelican::messages.wake_cooldown'), trans('lazy-pelican::messages.wake_cooldown_help')),
            $number('wake_max_per_hour', 1, trans('lazy-pelican::messages.wake_max_per_hour'), trans('lazy-pelican::messages.wake_max_per_hour_help')),
            $number('agent_retention_days', 1, trans('lazy-pelican::messages.agent_retention_days'), trans('lazy-pelican::messages.agent_retention_days_help')),
        ];
    }

    public function saveSettings(array $data): void
    {
        $this->writeToEnvironment([
            'LAZY_PELICAN_DEFAULT_IDLE_MINUTES' => $data['default_idle_minutes'],
            'LAZY_PELICAN_DEFAULT_GRACE_MINUTES' => $data['default_grace_minutes'],
            'LAZY_PELICAN_DETECTOR_PACKETS' => $data['detector_packets'],
            'LAZY_PELICAN_DETECTOR_WINDOW' => $data['detector_window'],
            'LAZY_PELICAN_DETECTOR_HOLDOFF' => $data['detector_holdoff'],
            'LAZY_PELICAN_WAKE_COOLDOWN' => $data['wake_cooldown'],
            'LAZY_PELICAN_WAKE_MAX_PER_HOUR' => $data['wake_max_per_hour'],
            'LAZY_PELICAN_AGENT_RETENTION_DAYS' => $data['agent_retention_days'],
        ]);

        Notification::make()
            ->title(trans('lazy-pelican::messages.settings_saved'))
            ->success()
            ->send();
    }
}
