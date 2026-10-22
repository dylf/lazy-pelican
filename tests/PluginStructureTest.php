<?php

namespace Dylf\LazyPelican\Tests;

use PHPUnit\Framework\TestCase;

/**
 * Static checks on the plugin's files. They don't run plugin code or boot the panel; they catch the kind of
 * mistake that is easy to make when renaming things (namespaces, translation keys, config names, ids).
 */
class PluginStructureTest extends TestCase
{
    private string $root;

    /** @var array<string, mixed> */
    private array $manifest;

    protected function setUp(): void
    {
        $this->root = dirname(__DIR__);
        $this->manifest = json_decode((string) file_get_contents($this->root . '/plugin.json'), true, 512, JSON_THROW_ON_ERROR);
    }

    /** @return string[] */
    private function phpFiles(string $dir): array
    {
        $files = [];
        $iterator = new \RecursiveIteratorIterator(new \RecursiveDirectoryIterator($this->root . '/' . $dir, \FilesystemIterator::SKIP_DOTS));

        foreach ($iterator as $file) {
            if ($file->isFile() && $file->getExtension() === 'php') {
                $files[] = $file->getPathname();
            }
        }

        sort($files);

        return $files;
    }

    private function allSourceCode(): string
    {
        return implode("\n", array_map('file_get_contents', $this->phpFiles('src')));
    }

    private function id(): string
    {
        return $this->manifest['id'];
    }

    // --- plugin.json ---------------------------------------------------------------------------

    public function test_manifest_has_the_required_fields(): void
    {
        foreach (['id', 'name', 'author', 'category', 'namespace', 'class'] as $field) {
            $this->assertArrayHasKey($field, $this->manifest, "plugin.json is missing '$field'");
            $this->assertNotSame('', $this->manifest[$field], "plugin.json '$field' is empty");
        }

        $this->assertContains($this->manifest['category'], ['plugin', 'theme', 'language']);
    }

    public function test_manifest_has_no_meta_key(): void
    {
        // The panel adds "meta" itself when installing; it must not be shipped (same check as pelican/plugins CI).
        $this->assertArrayNotHasKey('meta', $this->manifest);
    }

    public function test_plugin_id_is_a_valid_slug(): void
    {
        // Same rule the panel applies when importing a plugin.
        $this->assertMatchesRegularExpression('/^[a-z0-9][a-z0-9._-]*$/', $this->id());
    }

    public function test_main_class_file_matches_the_manifest(): void
    {
        $file = $this->root . '/src/' . $this->manifest['class'] . '.php';
        $this->assertFileExists($file);

        $code = (string) file_get_contents($file);
        $this->assertStringContainsString('namespace ' . $this->manifest['namespace'] . ';', $code);
        $this->assertMatchesRegularExpression('/class\s+' . preg_quote($this->manifest['class'], '/') . '\b/', $code);
    }

    public function test_main_class_reports_the_manifest_id(): void
    {
        $code = (string) file_get_contents($this->root . '/src/' . $this->manifest['class'] . '.php');

        $this->assertMatchesRegularExpression("/function getId\(\): string\s*\{\s*return '" . preg_quote($this->id(), '/') . "';/", $code);
    }

    public function test_config_file_is_named_after_the_plugin_id(): void
    {
        $this->assertFileExists($this->root . '/config/' . $this->id() . '.php');
    }

    // --- PHP files -----------------------------------------------------------------------------

    public function test_every_php_file_has_valid_syntax(): void
    {
        foreach ($this->phpFiles('src') as $file) {
            try {
                token_get_all((string) file_get_contents($file), TOKEN_PARSE);
            } catch (\ParseError $e) {
                $this->fail("Syntax error in $file: " . $e->getMessage() . ' on line ' . $e->getLine());
            }
        }

        $this->addToAssertionCount(1);
    }

    public function test_every_namespace_matches_its_directory(): void
    {
        foreach ($this->phpFiles('src') as $file) {
            $relative = substr($file, strlen($this->root . '/src/'));
            $dir = dirname($relative);
            $expected = $this->manifest['namespace'] . ($dir === '.' ? '' : '\\' . str_replace('/', '\\', $dir));

            preg_match('/^namespace\s+([^;]+);/m', (string) file_get_contents($file), $match);
            $this->assertSame($expected, $match[1] ?? null, "Wrong namespace in $relative");
        }
    }

    public function test_every_import_of_our_own_namespace_resolves_to_a_file(): void
    {
        $prefix = $this->manifest['namespace'] . '\\';

        foreach ($this->phpFiles('src') as $file) {
            preg_match_all('/^use\s+(' . preg_quote($prefix, '/') . '[^;\s]+);/m', (string) file_get_contents($file), $matches);

            foreach ($matches[1] as $import) {
                $path = $this->root . '/src/' . str_replace('\\', '/', substr($import, strlen($prefix))) . '.php';
                $this->assertFileExists($path, "$file imports $import but that file does not exist");
            }
        }
    }

    // --- translations --------------------------------------------------------------------------

    /** @return array<string, array<string, string>> */
    private function langFiles(): array
    {
        $files = [];

        foreach (glob($this->root . '/lang/en/*.php') as $file) {
            $files[basename($file, '.php')] = require $file;
        }

        return $files;
    }

    public function test_every_translation_key_used_in_the_source_exists(): void
    {
        $lang = $this->langFiles();

        preg_match_all("/['\"]" . preg_quote($this->id(), '/') . "::([a-z_]+)\.([a-z_]+)['\"]/", $this->allSourceCode(), $matches, PREG_SET_ORDER);
        $this->assertNotEmpty($matches, 'expected the source to use translation keys');

        foreach ($matches as [$full, $file, $key]) {
            $this->assertArrayHasKey($file, $lang, "lang/en/$file.php is missing (used by $full)");
            $this->assertArrayHasKey($key, $lang[$file], "$full is used in the source but missing from lang/en/$file.php");
        }
    }

    public function test_subuser_permission_group_has_all_its_labels(): void
    {
        $lang = $this->langFiles();

        preg_match_all("/registerCustomPermissions\('([^']+)',\s*\[([^\]]*)\],\s*'" . preg_quote($this->id(), '/') . "::([a-z_]+)'/", $this->allSourceCode(), $matches, PREG_SET_ORDER);
        $this->assertNotEmpty($matches, 'expected a registerCustomPermissions() call');

        foreach ($matches as [, $group, $permissions, $langFile]) {
            $this->assertArrayHasKey($langFile, $lang);

            $keys = ["{$group}_title", "{$group}_desc"];
            foreach (array_map(fn ($p) => trim($p, " '\""), explode(',', $permissions)) as $permission) {
                $keys[] = "{$group}_{$permission}";
            }

            foreach ($keys as $key) {
                $this->assertArrayHasKey($key, $lang[$langFile], "lang/en/$langFile.php is missing '$key' for the '$group' permission group");
            }
        }
    }

    // --- config / env --------------------------------------------------------------------------

    /** @return array<string, mixed> */
    private function config(): array
    {
        return require $this->root . '/config/' . $this->id() . '.php';
    }

    public function test_every_config_key_used_in_the_source_exists(): void
    {
        $config = $this->config();

        preg_match_all("/config\('" . preg_quote($this->id(), '/') . "\.([a-z_]+)'/", $this->allSourceCode(), $matches);
        $this->assertNotEmpty($matches[1], 'expected the source to read config keys');

        foreach (array_unique($matches[1]) as $key) {
            $this->assertArrayHasKey($key, $config, "config('{$this->id()}.$key') is read but not defined in config/{$this->id()}.php");
        }
    }

    public function test_env_variables_are_prefixed_with_the_plugin_id(): void
    {
        $prefix = strtoupper(str_replace('-', '_', $this->id())) . '_';

        preg_match_all("/env\('([A-Z0-9_]+)'/", (string) file_get_contents($this->root . '/config/' . $this->id() . '.php'), $matches);
        $this->assertNotEmpty($matches[1]);

        foreach ($matches[1] as $variable) {
            $this->assertStringStartsWith($prefix, $variable, 'Pelican asks plugins to prefix env variables with the plugin id');
        }
    }

    public function test_settings_page_writes_the_same_env_variables_the_config_reads(): void
    {
        $prefix = strtoupper(str_replace('-', '_', $this->id())) . '_';

        preg_match_all("/env\('(" . $prefix . "[A-Z0-9_]+)'/", (string) file_get_contents($this->root . '/config/' . $this->id() . '.php'), $read);
        preg_match_all("/'(" . $prefix . "[A-Z0-9_]+)'\s*=>/", (string) file_get_contents($this->root . '/src/' . $this->manifest['class'] . '.php'), $written);

        $this->assertNotEmpty($written[1], 'expected saveSettings() to write env variables');

        foreach ($written[1] as $variable) {
            $this->assertContains($variable, $read[1], "saveSettings() writes $variable but config never reads it");
        }
    }

    // --- scheduling / database -----------------------------------------------------------------

    public function test_the_scheduled_command_exists(): void
    {
        preg_match_all("/->command\('([^']+)'\)/", $this->allSourceCode(), $scheduled);
        $this->assertNotEmpty($scheduled[1], 'expected a scheduled command');

        preg_match_all('/\$signature\s*=\s*\'([^\'\s]+)/', $this->allSourceCode(), $signatures);

        foreach ($scheduled[1] as $command) {
            $this->assertContains($command, $signatures[1], "The scheduler runs '$command' but no command class has that signature");
        }
    }

    public function test_each_model_has_a_matching_migration_table(): void
    {
        $migrations = '';
        foreach ($this->phpFiles('database/migrations') as $file) {
            $migrations .= file_get_contents($file);
        }
        preg_match_all("/Schema::create\('([^']+)'/", $migrations, $created);

        foreach (glob($this->root . '/src/Models/*.php') as $file) {
            $code = (string) file_get_contents($file);

            if (preg_match('/\$table\s*=\s*\'([^\']+)\'/', $code, $explicit)) {
                $expected = $explicit[1];
            } else {
                // Laravel's default: snake_case plural of the class name.
                $expected = strtolower(preg_replace('/(?<!^)[A-Z]/', '_$0', basename($file, '.php'))) . 's';
            }

            $this->assertContains($expected, $created[1], basename($file) . " expects table '$expected' but no migration creates it");
        }
    }
}
