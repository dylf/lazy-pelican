<?php

namespace Dylf\LazyPelican\Filament\Server\Pages;

use App\Facades\Activity;
use App\Filament\Server\Pages\ServerFormPage;
use App\Models\Server;
use Dylf\LazyPelican\Models\IdleShutdownSetting;
use Dylf\LazyPelican\Services\IdleShutdownService;
use Exception;
use Filament\Facades\Filament;
use Filament\Forms\Components\TextInput;
use Filament\Forms\Components\Toggle;
use Filament\Notifications\Notification;
use Filament\Schemas\Components\Section;
use Filament\Schemas\Components\Text;
use Filament\Schemas\Schema;

class IdleShutdownPage extends ServerFormPage
{
    protected static string|\BackedEnum|null $navigationIcon = 'tabler-moon';

    protected static ?string $slug = 'idle-shutdown';

    protected static ?int $navigationSort = 31;

    public static function canAccess(): bool
    {
        return parent::canAccess()
            && user()?->can('idle_shutdown.update', Filament::getTenant());
    }

    protected function authorizeAccess(): void
    {
        abort_unless(user()?->can('idle_shutdown.update', $this->getRecord()), 403);
    }

    public static function getNavigationLabel(): string
    {
        return trans('lazy-pelican::messages.title');
    }

    public function getTitle(): string
    {
        return trans('lazy-pelican::messages.title');
    }

    protected function setting(): IdleShutdownSetting
    {
        return IdleShutdownSetting::firstOrNew(['server_id' => $this->getRecord()->id]);
    }

    protected function fillForm(): void
    {
        $setting = $this->setting();

        $this->form->fill([
            'enabled' => $setting->enabled,
            'idle_minutes' => $setting->idle_minutes,
            'grace_minutes' => $setting->grace_minutes,
        ]);
    }

    public function form(Schema $schema): Schema
    {
        /** @var Server $server */
        $server = $this->getRecord();

        $hasPlayerCounter = IdleShutdownService::playerCounterAvailable();

        // @phpstan-ignore property.notFound
        $hasQuery = $hasPlayerCounter && $server->egg->gameQuery()->exists();

        return $schema
            ->statePath('data')
            ->components([
                Section::make([
                    Text::make(trans('lazy-pelican::messages.needs_player_counter'))
                        ->color('danger')
                        ->visible(!$hasPlayerCounter),

                    Text::make(trans('lazy-pelican::messages.needs_query'))
                        ->color('warning')
                        ->visible($hasPlayerCounter && !$hasQuery),

                    Toggle::make('enabled')
                        ->disabled(!$hasPlayerCounter)
                        ->label(trans('lazy-pelican::messages.enabled'))
                        ->helperText(trans('lazy-pelican::messages.enabled_hint'))
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
            ]);
    }

    public function save(): void
    {
        abort_unless(user()?->can('idle_shutdown.update', $this->getRecord()), 403);

        try {
            $data = $this->form->getState();

            $setting = $this->setting();
            $setting->fill([
                'enabled' => (bool) ($data['enabled'] ?? false),
                'idle_minutes' => filled($data['idle_minutes'] ?? null) ? (int) $data['idle_minutes'] : null,
                'grace_minutes' => filled($data['grace_minutes'] ?? null) ? (int) $data['grace_minutes'] : null,
            ]);
            $setting->save();

            // Changing settings restarts the idle clock.
            $setting->resetState();

            Activity::event('server:idle-shutdown.update')
                ->property('enabled', $setting->enabled)
                ->log();

            Notification::make()
                ->title(trans('lazy-pelican::messages.saved'))
                ->success()
                ->send();
        } catch (Exception $exception) {
            report($exception);

            Notification::make()
                ->title('Failed to save')
                ->body($exception->getMessage())
                ->danger()
                ->send();
        }
    }
}
