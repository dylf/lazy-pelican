<?php

namespace Dylf\LazyPelican\Console\Commands;

use Dylf\LazyPelican\Services\IdleShutdownService;
use Illuminate\Console\Command;

class CheckIdleServersCommand extends Command
{
    protected $signature = 'lazy-pelican:check';

    protected $description = 'Stop servers that have had no players for longer than their idle timeout';

    public function handle(IdleShutdownService $service): int
    {
        if (!IdleShutdownService::playerCounterAvailable()) {
            $this->warn(trans('lazy-pelican::messages.needs_player_counter_admin'));
        }

        $results = $service->checkAll();

        foreach ($results as $serverId => $decision) {
            if (config('lazy-pelican.verbose') || $this->output->isVerbose()) {
                $this->line("server $serverId: $decision");
            }
        }

        return self::SUCCESS;
    }
}
