<?php

namespace Dylf\LazyPelican\Tests;

use Dylf\LazyPelican\Support\NodeMatcher;
use PHPUnit\Framework\TestCase;

class NodeMatcherTest extends TestCase
{
    public function test_empty_key_matches_any_node(): void
    {
        $this->assertTrue(NodeMatcher::matches(null, 2, 'node', 'node.example.com'));
        $this->assertTrue(NodeMatcher::matches('', 2, 'node', 'node.example.com'));
        $this->assertTrue(NodeMatcher::matches('   ', 2, 'node', 'node.example.com'));
    }

    public function test_matches_by_id_name_or_fqdn_ignoring_case(): void
    {
        $this->assertTrue(NodeMatcher::matches('2', 2, 'node', 'node.example.com'));
        $this->assertTrue(NodeMatcher::matches('Node', 2, 'node', 'node.example.com'));
        $this->assertTrue(NodeMatcher::matches(' NODE.example.COM ', 2, 'node', 'node.example.com'));
    }

    public function test_does_not_match_a_different_node(): void
    {
        $this->assertFalse(NodeMatcher::matches('3', 2, 'node', 'node.example.com'));
        $this->assertFalse(NodeMatcher::matches('other', 2, 'node', 'node.example.com'));
        $this->assertFalse(NodeMatcher::matches('node.example', 2, 'node', 'node.example.com'), 'partial FQDN must not match');
    }

    public function test_id_is_not_matched_as_a_prefix(): void
    {
        $this->assertFalse(NodeMatcher::matches('20', 2, 'node', 'n'));
        $this->assertFalse(NodeMatcher::matches('2', 12, 'node', 'n'));
    }
}
