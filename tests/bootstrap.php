<?php

// The checks in this suite read the plugin's files and exercise pure-logic classes; they don't boot the panel.
// config/*.php calls Laravel's env(), so provide a stand-in that returns the default.
if (!function_exists('env')) {
    function env(string $key, mixed $default = null): mixed
    {
        return $default;
    }
}

// Minimal PSR-4 autoloader for the plugin's own classes that have no panel dependencies.
spl_autoload_register(function (string $class): void {
    $prefix = 'Dylf\\LazyPelican\\';

    if (!str_starts_with($class, $prefix)) {
        return;
    }

    $file = __DIR__ . '/../src/' . str_replace('\\', '/', substr($class, strlen($prefix))) . '.php';

    if (is_file($file)) {
        require $file;
    }
});
