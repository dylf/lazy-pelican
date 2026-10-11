<?php

namespace Dylf\LazyPelican\Providers;

use App\Models\Subuser;
use Dylf\LazyPelican\Services\IdleShutdownService;
use Illuminate\Console\Scheduling\Schedule;
use Illuminate\Support\ServiceProvider;

class LazyPelicanPluginProvider extends ServiceProvider
{
    public function register(): void
    {
        $this->app->singleton(IdleShutdownService::class);

        // Lets owners grant subusers the right to change the Lazy Pelican settings (idle shutdown and wake on connect).
        Subuser::registerCustomPermissions('lazy_pelican', ['update'], 'lazy-pelican::permissions', 'tabler-moon');
    }

    public function boot(): void
    {
        $this->callAfterResolving(Schedule::class, function (Schedule $schedule) {
            $schedule->command('lazy-pelican:check')
                ->everyMinute()
                ->withoutOverlapping(5);
        });
    }
}
