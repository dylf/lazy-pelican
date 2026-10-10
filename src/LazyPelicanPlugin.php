<?php

namespace Dylf\LazyPelican;

use App\Contracts\Plugins\HasPluginSettings;
use App\Traits\EnvironmentWriterTrait;
use Filament\Contracts\Plugin;
use Filament\Forms\Components\TextInput;
use Dylf\LazyPelican\Services\IdleShutdownService;
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

        return [
            Text::make(trans($available ? 'lazy-pelican::messages.player_counter_ok_admin' : 'lazy-pelican::messages.needs_player_counter_admin'))
                ->color($available ? 'success' : 'danger'),
            TextInput::make('default_idle_minutes')
                ->label(trans('lazy-pelican::messages.default_idle_minutes'))
                ->numeric()
                ->minValue(1)
                ->required()
                ->default(fn () => config('lazy-pelican.default_idle_minutes')),
            TextInput::make('default_grace_minutes')
                ->label(trans('lazy-pelican::messages.default_grace_minutes'))
                ->numeric()
                ->minValue(0)
                ->required()
                ->default(fn () => config('lazy-pelican.default_grace_minutes')),
        ];
    }

    public function saveSettings(array $data): void
    {
        $this->writeToEnvironment([
            'LAZY_PELICAN_DEFAULT_IDLE_MINUTES' => $data['default_idle_minutes'],
            'LAZY_PELICAN_DEFAULT_GRACE_MINUTES' => $data['default_grace_minutes'],
        ]);

        Notification::make()
            ->title(trans('lazy-pelican::messages.settings_saved'))
            ->success()
            ->send();
    }
}
