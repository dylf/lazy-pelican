<?php

return [
    'title' => 'Lazy Pelican',
    'saved' => 'Lazy Pelican settings saved',
    'save_failed' => 'Failed to save',

    // Idle shutdown
    'idle_section' => 'Stop when empty',
    'idle_section_hint' => 'Stops the server after it has had no players for the time below.',
    'enabled' => 'Stop server when empty',
    'enabled_hint' => 'Players (or you) start it again from the panel, or automatically with wake on connect.',
    'idle_minutes' => 'Minutes with no players before stopping',
    'grace_minutes' => 'Startup grace period (minutes)',
    'grace_hint' => 'After the server starts, wait this long before counting idle time, so players have time to join.',
    'needs_query' => 'This server\'s egg has no Game Query configured in the Player Counter plugin, so player counts can\'t be read. Idle shutdown will never stop it until one is set up.',
    'needs_player_counter' => 'The Player Counter plugin is required. Install and enable it (Admin -> Plugins), then create a Game Query for this server\'s egg. Until then Idle Shutdown cannot read player counts and will not stop this server.',
    'needs_player_counter_admin' => 'Player Counter plugin not found. Idle Shutdown requires the Player Counter plugin to be installed AND enabled; without it no server will ever be stopped.',
    'player_counter_ok_admin' => 'Player Counter plugin detected.',
    'player_counter_missing_log' => 'lazy-pelican: the Player Counter plugin is not installed/enabled, so :count server(s) with idle shutdown enabled cannot be checked. No servers were stopped.',

    // Wake on connect
    'wake_section' => 'Start when someone tries to join',
    'wake_section_hint' => 'Needs the lazy-pelican-agent running on this server\'s node. It watches for players trying to connect while the server is stopped and starts it. The first attempt will still time out while the server boots, so players retry.',
    'wake_enabled' => 'Wake on connect',
    'wake_ports' => 'Game port(s) to watch',
    'wake_ports_hint' => 'The port players connect to. Leave empty to use this server\'s primary allocation. Do not add a query port.',
    'wake_advanced' => 'Advanced match rules',
    'wake_advanced_hint' => 'Only count packets that look like this game\'s join traffic. Leave empty unless the server is being woken by scanners.',
    'wake_prefix' => 'Payload starts with (hex)',
    'wake_length' => 'Payload length (bytes)',

    // Status
    'status_section' => 'Status',
    'last_woken' => 'Last woken by the agent: :when',
    'last_stopped' => 'Last stopped for being empty: :when',
    'never' => 'never',
    'agent_none' => 'No agent has reported in for this node. Wake on connect needs the lazy-pelican-agent running on the Wings host. See Admin -> Lazy Pelican for setup.',
    'agent_line' => 'Agent :host (v:version) on :node: :state, last heard :when',
    'agent_capture_failed' => 'Agent :host cannot capture packets: :error',
    'agent_cannot_start' => 'The agent\'s user ":user" cannot start this server. Add it as a subuser with the Start permission, otherwise this server won\'t be woken.',
    'agent_not_watching' => 'Agent :host is connected but is not watching this server yet. It refreshes its config every minute.',
    'wake_off_but_agents' => 'Wake on connect is off for this server.',
    'state_connected' => 'connected',
    'state_stale' => 'not responding',
    'state_offline' => 'offline',

    // Admin settings (plugin list)
    'default_idle_minutes' => 'Default idle minutes',
    'default_grace_minutes' => 'Default grace minutes',
    'detector_packets' => 'Join attempt: packets needed',
    'detector_window' => 'Join attempt: within (seconds)',
    'detector_holdoff' => 'After a detection, ignore the server for (seconds)',
    'wake_cooldown' => 'After waking a server, don\'t wake it again for (minutes)',
    'wake_max_per_hour' => 'Maximum wakes per server per hour',
    'settings_saved' => 'Lazy Pelican defaults saved',
    'default_idle_minutes_help' => 'How long a server must have no players before Idle Shutdown stops it. Each server can override this on its own Lazy Pelican page.',
    'default_grace_minutes_help' => 'After a server starts, wait this long before counting idle time, so players have time to join. Valheim needs about a minute to load its world.',
    'detector_packets_help' => 'How many packets from one address count as someone trying to join. A scanner usually sends one; a player\'s game keeps retrying. Higher is stricter, lower reacts faster.',
    'detector_window_help' => 'The packets above must arrive within this many seconds.',
    'detector_holdoff_help' => 'Stops one player\'s repeated retries from being counted as many separate attempts.',
    'wake_cooldown_help' => 'Keep this longer than the server takes to start, so a player\'s retries while it boots can\'t start it twice.',
    'wake_max_per_hour_help' => 'A safety cap: the agent never wakes one server more than this many times an hour, so a scanner can\'t make it start over and over.',
    'agent_refresh_note' => 'The wake settings below are sent to the lazy-pelican-agent, which picks up changes within a minute.',

    // Admin page
    'admin_title' => 'Lazy Pelican',
    'admin_agents' => 'Agents',
    'admin_agents_empty' => 'No agent has reported in yet.',
    'admin_setup' => 'Set up the wake agent',
    'admin_servers' => 'Servers with wake on connect',
    'admin_servers_empty' => 'No server has wake on connect enabled. Turn it on from a server\'s Lazy Pelican page.',
];
