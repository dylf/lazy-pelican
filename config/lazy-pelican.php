<?php

return [
    // Minutes with zero players before a server is stopped.
    'default_idle_minutes' => (int) env('LAZY_PELICAN_DEFAULT_IDLE_MINUTES', 15),

    // Minutes after a server is first seen running before idle time starts counting.
    // Gives players time to join after a (manual) start, e.g. Valheim takes a while to load the world.
    'default_grace_minutes' => (int) env('LAZY_PELICAN_DEFAULT_GRACE_MINUTES', 10),

    // Log decisions (every check) at info level. Useful while testing.
    'verbose' => (bool) env('LAZY_PELICAN_VERBOSE', false),
];
