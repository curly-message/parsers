<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

use Opis\JsonSchema\Errors\ErrorFormatter;
use Opis\JsonSchema\Validator;

/**
 * The conformance set as a release ships it: the fixture files, sorted by
 * name, each held to the schema and to the manifest before a case is read,
 * and the catalogue of defects a runner is audited against.
 */
final class Set
{
    /** The versioned identifier of the format this runner reads. */
    public const FORMAT = 'curly-message-3';

    /**
     * @param list<Fixture> $fixtures
     * @param list<DefectEntry> $defects
     */
    private function __construct(
        public readonly string $root,
        public readonly string $version,
        public readonly array $fixtures,
        public readonly array $defects,
        private readonly bool $validate,
    ) {}

    /** The set `conformance/set.json` pins, as `composer fetch-set` installed it. */
    public static function pinned(bool $validate = true): self
    {
        $home = dirname(__DIR__);
        $pin = Fetch::pin("{$home}/set.json");

        if (!Fetch::installed("{$home}/.set", $pin['version'], $pin['sha256'])) {
            throw new \RuntimeException("The conformance set {$pin['version']} is not installed; run `composer fetch-set`.");
        }

        $set = self::open(Fetch::root("{$home}/.set", $pin['version']), $validate);

        if ($set->version !== $pin['version']) {
            throw new \RuntimeException("The installed set is version {$set->version}, not the pinned {$pin['version']}.");
        }

        return $set;
    }

    /**
     * The set under a directory holding `index.json`, `fixtures/`, `schema/`
     * and `defects.json`. A file whose format this runner does not read is
     * refused, as is one the schema or the manifest disagrees with: a set a
     * runner could not read is not a set an implementation passed.
     */
    public static function open(string $root, bool $validate = true): self
    {
        $manifest = self::json("{$root}/index.json");
        self::readable($manifest, 'index.json');
        $version = $manifest->version ?? null;

        if (!is_string($version)) {
            throw new \RuntimeException('The manifest names no version of the set.');
        }

        $set = new self($root, $version, [], [], $validate);
        $fixtures = $set->load("{$root}/fixtures");
        self::agrees($manifest, $fixtures);

        $catalogue = self::json("{$root}/defects.json");
        self::readable($catalogue, 'defects.json');
        $set->valid($catalogue, 'defect', 'defects.json');

        return new self($root, $version, $fixtures, self::entries($catalogue), $validate);
    }

    /**
     * The fixture files of a directory, sorted by name, each held to this
     * set's schema: what the runner's `fixtures` option takes, for a set under
     * development as for the shipped one.
     *
     * @return list<Fixture>
     */
    public function load(string $directory): array
    {
        $names = [];

        $listed = scandir($directory);

        if ($listed === false) {
            throw new \RuntimeException("{$directory} cannot be read.");
        }

        foreach ($listed as $name) {
            if (str_ends_with($name, '.json') && is_file("{$directory}/{$name}")) {
                $names[] = $name;
            }
        }

        sort($names, SORT_STRING);
        $fixtures = [];

        foreach ($names as $name) {
            $file = self::json("{$directory}/{$name}");
            self::readable($file, $name);
            $this->valid($file, 'fixture', $name);
            $fixtures[] = new Fixture($name, $file);
        }

        return $fixtures;
    }

    private static function json(string $path): \stdClass
    {
        $text = @file_get_contents($path);

        if ($text === false) {
            throw new \RuntimeException("{$path} cannot be read.");
        }

        try {
            $value = json_decode($text, false, 512, JSON_THROW_ON_ERROR);
        } catch (\JsonException $error) {
            // A PHP object has no property whose name begins with NUL, so a
            // key spelled that way is refused here rather than read as another.
            throw new \RuntimeException("{$path} is not JSON this runner can read: {$error->getMessage()}.", 0, $error);
        }

        if (!$value instanceof \stdClass) {
            throw new \RuntimeException("{$path} holds no object.");
        }

        return $value;
    }

    private static function readable(\stdClass $file, string $name): void
    {
        $format = $file->format ?? null;

        if ($format !== self::FORMAT) {
            throw new \RuntimeException("{$name} targets the format " . json_encode($format) . '; this runner reads ' . self::FORMAT . '.');
        }
    }

    private function valid(\stdClass $value, string $schema, string $name): void
    {
        if (!$this->validate) {
            return;
        }

        $validator = new Validator();
        $validator->setMaxErrors(1);
        $result = $validator->validate($value, self::json("{$this->root}/schema/{$schema}.schema.json"));
        $error = $result->error();

        if ($error !== null) {
            $messages = (new ErrorFormatter())->format($error, false);
            $where = (string) array_key_first($messages);
            $why = $messages[$where] ?? null;

            throw new \RuntimeException("{$name} does not validate against {$schema}.schema.json: at {$where}, " . (is_string($why) ? $why : 'no reason given') . '.');
        }
    }

    /** @param list<Fixture> $fixtures */
    private static function agrees(\stdClass $manifest, array $fixtures): void
    {
        $listed = [];

        foreach (is_array($manifest->files ?? null) ? $manifest->files : [] as $entry) {
            if ($entry instanceof \stdClass && is_string($entry->path ?? null)) {
                $listed[$entry->path] = $entry;
            }
        }

        $shipped = array_map(static fn(Fixture $fixture): string => "fixtures/{$fixture->name}", $fixtures);

        if (array_keys($listed) !== $shipped) {
            throw new \RuntimeException('The manifest lists ' . implode(', ', array_keys($listed)) . '; the set holds ' . implode(', ', $shipped) . '.');
        }

        foreach ($fixtures as $fixture) {
            $entry = $listed["fixtures/{$fixture->name}"];
            $file = $fixture->file;
            $cases = is_array($file->cases ?? null) ? count($file->cases) : -1;
            $stated = [$entry->level ?? null, $entry->kind ?? null, $entry->section ?? null, $entry->cases ?? null];
            $held = [$file->level ?? null, $file->kind ?? null, $file->section ?? null, $cases];

            if ($stated !== $held) {
                throw new \RuntimeException("The manifest states {$fixture->name} as " . json_encode($stated) . '; the file is ' . json_encode($held) . '.');
            }
        }
    }

    /** @return list<DefectEntry> */
    private static function entries(\stdClass $catalogue): array
    {
        $entries = [];

        foreach (is_array($catalogue->defects ?? null) ? $catalogue->defects : [] as $defect) {
            if (!$defect instanceof \stdClass) {
                throw new \RuntimeException('defects.json holds a defect that is not an object.');
            }

            $id = $defect->id ?? null;
            $description = $defect->description ?? null;
            $document = $defect->document ?? null;
            $section = $defect->section ?? null;
            $expects = $defect->expects ?? null;

            if (!is_string($id) || !is_string($description) || !in_array($expects, ['error', 'fail', 'skip', 'unobserved'], true) || ($document !== null && !is_string($document)) || ($section !== null && !is_string($section))) {
                throw new \RuntimeException('defects.json holds a defect this runner cannot read: ' . json_encode($id) . '.');
            }

            $entries[] = new DefectEntry($id, $description, $document, $section, $expects);
        }

        return $entries;
    }
}
