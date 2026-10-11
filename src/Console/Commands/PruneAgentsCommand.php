<?php

namespace Dylf\LazyPelican\Console\Commands;

use Dylf\LazyPelican\Models\LazyPelicanAgent;
use Illuminate\Console\Command;

class PruneAgentsCommand extends Command
{
    protected $signature = 'lazy-pelican:prune-agents';

    protected $description = 'Forget lazy-pelican-agents that have not reported in for a long time';

    public function handle(): int
    {
        $days = (int) config('lazy-pelican.agent_retention_days');
        $removed = LazyPelicanAgent::prune($days);

        if ($removed > 0 || $this->output->isVerbose()) {
            $this->line("Removed $removed agent(s) not heard from for $days day(s).");
        }

        return self::SUCCESS;
    }
}
