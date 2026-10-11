<?php

return [
    // Idle shutdown ------------------------------------------------------------------------------------------

    // Minutes with zero players before a server is stopped.
    'default_idle_minutes' => (int) env('LAZY_PELICAN_DEFAULT_IDLE_MINUTES', 15),

    // Minutes after a server is first seen running before idle time starts counting.
    // Gives players time to join after a (manual) start, e.g. Valheim takes a while to load the world.
    'default_grace_minutes' => (int) env('LAZY_PELICAN_DEFAULT_GRACE_MINUTES', 10),

    // Log decisions (every check) at info level. Useful while testing.
    'verbose' => (bool) env('LAZY_PELICAN_VERBOSE', false),

    // Wake on connect: handed to the lazy-pelican-agent, which applies them ---------------------------------------------

    // A burst of this many non-query UDP packets from one address...
    'detector_packets' => (int) env('LAZY_PELICAN_DETECTOR_PACKETS', 3),

    // ...within this many seconds counts as a player trying to join.
    'detector_window' => (int) env('LAZY_PELICAN_DETECTOR_WINDOW', 5),

    // After a join attempt is detected, ignore that server for this many seconds.
    'detector_holdoff' => (int) env('LAZY_PELICAN_DETECTOR_HOLDOFF', 30),

    // After waking a server, don't wake it again for this many minutes.
    'wake_cooldown' => (int) env('LAZY_PELICAN_WAKE_COOLDOWN', 5),

    // Never wake one server more than this many times an hour (stops scanners making it start over and over).
    'wake_max_per_hour' => (int) env('LAZY_PELICAN_WAKE_MAX_PER_HOUR', 6),
];
