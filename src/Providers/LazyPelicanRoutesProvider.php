<?php

namespace Dylf\LazyPelican\Providers;

use Illuminate\Foundation\Support\Providers\RouteServiceProvider;
use Illuminate\Support\Facades\Route;

class LazyPelicanRoutesProvider extends RouteServiceProvider
{
    public function boot(): void
    {
        $this->routes(function () {
            // Same stack the built-in client API uses, so the agent authenticates with an ordinary client API key.
            Route::middleware(['api', 'client-api', 'throttle:api.client'])
                ->prefix('/api/client')
                ->scopeBindings()
                ->group(plugin_path('lazy-pelican', 'routes/api-client.php'));
        });
    }
}
