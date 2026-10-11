<?php

use Dylf\LazyPelican\Http\Controllers\Api\Client\AgentController;
use Illuminate\Support\Facades\Route;

Route::prefix('/lazy-pelican/agent')->group(function () {
    Route::get('/config', [AgentController::class, 'config']);
    Route::post('/heartbeat', [AgentController::class, 'heartbeat']);
    Route::post('/wake', [AgentController::class, 'wake']);
});
