<?php

namespace Dylf\LazyPelican\Support;

/**
 * Decides whether an agent's `node` setting refers to a given node. No panel classes, so it's unit-testable.
 */
final class NodeMatcher
{
    /**
     * Empty means "any node". Otherwise the key may be the node's id, name or FQDN (case-insensitive).
     */
    public static function matches(?string $key, int $id, string $name, string $fqdn): bool
    {
        $key = strtolower(trim((string) $key));

        if ($key === '') {
            return true;
        }

        return $key === (string) $id || $key === strtolower($name) || $key === strtolower($fqdn);
    }
}
