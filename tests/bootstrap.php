<?php

// The checks in this suite read the plugin's files; they don't boot the panel.
// config/*.php calls Laravel's env(), so provide a stand-in that returns the default.
if (!function_exists('env')) {
    function env(string $key, mixed $default = null): mixed
    {
        return $default;
    }
}
