<?php

namespace Dylf\LazyPelican\Support;

/**
 * One line of the admin page's "servers with wake on connect" table.
 */
final class WakeServerRow
{
    public function __construct(
        public readonly string $name,
        public readonly string $node,
        public readonly string $ports,
        public readonly string $lastWoken,
        public readonly ?string $problem,
    ) {}
}
