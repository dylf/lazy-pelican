<?php

return [
    'title' => 'Idle Shutdown',
    'enabled' => 'Stop server when empty',
    'enabled_hint' => 'Stops the server after it has had no players for the time below. Players (or you) start it again from the panel.',
    'idle_minutes' => 'Minutes with no players before stopping',
    'grace_minutes' => 'Startup grace period (minutes)',
    'grace_hint' => 'After the server starts, wait this long before counting idle time, so players have time to join.',
    'needs_query' => 'This server\'s egg has no Game Query configured in the Player Counter plugin, so player counts can\'t be read. Idle shutdown will never stop it until one is set up.',
    'needs_player_counter' => 'The Player Counter plugin is required. Install and enable it (Admin -> Plugins), then create a Game Query for this server\'s egg. Until then Idle Shutdown cannot read player counts and will not stop this server.',
    'needs_player_counter_admin' => 'Player Counter plugin not found. Idle Shutdown requires the Player Counter plugin to be installed AND enabled; without it no server will ever be stopped.',
    'player_counter_ok_admin' => 'Player Counter plugin detected.',
    'player_counter_missing_log' => 'lazy-pelican: the Player Counter plugin is not installed/enabled, so :count server(s) with idle shutdown enabled cannot be checked. No servers were stopped.',
    'saved' => 'Idle shutdown settings saved',
    'default_idle_minutes' => 'Default idle minutes',
    'default_grace_minutes' => 'Default grace minutes',
    'settings_saved' => 'Idle shutdown defaults saved',
];
