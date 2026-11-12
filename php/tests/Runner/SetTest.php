<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\Fixture;
use CurlyMessage\Conformance\Set;
use PHPUnit\Framework\TestCase;

final class SetTest extends TestCase
{
    private const CASE = ['id' => 'a/b', 'description' => 'D.', 'message' => 'm', 'expected' => ['output' => 'm']];

    private string $root;

    protected function setUp(): void
    {
        $this->root = sys_get_temp_dir() . '/curly-set-' . bin2hex(random_bytes(6));
        mkdir("{$this->root}/schema", 0o777, true);
        mkdir("{$this->root}/fixtures");

        foreach (['fixture', 'defect'] as $schema) {
            copy(Set::pinned(false)->root . "/schema/{$schema}.schema.json", "{$this->root}/schema/{$schema}.schema.json");
        }

        $this->write('fixtures/b.json', ['format' => Set::FORMAT, 'level' => 'core', 'section' => '8', 'cases' => [self::CASE]]);
        $this->write('fixtures/a.json', ['format' => Set::FORMAT, 'kind' => 'tree', 'section' => '6', 'cases' => [['id' => 'a/t', 'description' => 'D.', 'message' => 'm', 'expected' => [['type' => 'text', 'text' => 'm']]]]]);
        $this->write('fixtures/notes.txt', 'not a fixture');
        $this->write('index.json', ['format' => Set::FORMAT, 'version' => '9.0.0', 'files' => [
            ['path' => 'fixtures/a.json', 'kind' => 'tree', 'section' => '6', 'cases' => 1],
            ['path' => 'fixtures/b.json', 'level' => 'core', 'section' => '8', 'cases' => 1],
        ]]);
        $this->write('defects.json', ['format' => Set::FORMAT, 'defects' => [['id' => 'raises', 'description' => 'Raises.', 'section' => '14.3', 'expects' => 'fail']]]);
    }

    protected function tearDown(): void
    {
        foreach (['fixtures', 'schema', ''] as $directory) {
            $files = glob("{$this->root}/{$directory}/*.*");
            self::assertIsArray($files);
            array_map(unlink(...), $files);
        }

        rmdir("{$this->root}/fixtures");
        rmdir("{$this->root}/schema");
        rmdir($this->root);
    }

    public function testASetIsItsFixtureFilesByNameAndItsCatalogue(): void
    {
        $set = Set::open($this->root);

        self::assertSame('9.0.0', $set->version);
        self::assertSame(['a.json', 'b.json'], array_map(static fn(Fixture $fixture): string => $fixture->name, $set->fixtures));
        self::assertCount(1, $set->defects);
        self::assertSame(['raises', 'fail', null, '14.3'], [$set->defects[0]->id, $set->defects[0]->expects, $set->defects[0]->document, $set->defects[0]->section]);
    }

    public function testThePinnedSetIsTheVersionItPins(): void
    {
        $set = Set::pinned();

        self::assertSame('4.1.0', $set->version);
        self::assertCount(26, $set->defects);
    }

    public function testAManifestOfAnotherFormatIsRefused(): void
    {
        $this->write('index.json', ['format' => 'curly-message-2', 'version' => '9.0.0', 'files' => []]);

        $this->expectExceptionMessage('index.json targets the format "curly-message-2"; this runner reads curly-message-3.');

        Set::open($this->root);
    }

    public function testAFixtureOfAnotherFormatIsRefused(): void
    {
        $this->write('fixtures/b.json', ['level' => 'core', 'section' => '8', 'cases' => [self::CASE]]);

        $this->expectExceptionMessage('b.json targets the format null; this runner reads curly-message-3.');

        Set::open($this->root);
    }

    public function testAFixtureTheSchemaRefusesIsRefused(): void
    {
        $this->write('fixtures/b.json', ['format' => Set::FORMAT, 'level' => 'core', 'section' => '8', 'cases' => [array_diff_key(self::CASE, ['description' => true])]]);

        $this->expectExceptionMessageMatches('/^b\.json does not validate against fixture\.schema\.json: at .+\.$/');

        Set::open($this->root);
    }

    public function testASetUnderDevelopmentCanBeReadUnvalidated(): void
    {
        $this->write('fixtures/b.json', ['format' => Set::FORMAT, 'level' => 'core', 'section' => '8', 'cases' => [array_diff_key(self::CASE, ['description' => true])]]);

        self::assertCount(2, Set::open($this->root, false)->fixtures);
    }

    public function testAManifestListingOtherFilesIsRefused(): void
    {
        unlink("{$this->root}/fixtures/a.json");

        $this->expectExceptionMessage('The manifest lists fixtures/a.json, fixtures/b.json; the set holds fixtures/b.json.');

        Set::open($this->root);
    }

    public function testAManifestStatingAFileOtherwiseIsRefused(): void
    {
        $this->write('fixtures/b.json', ['format' => Set::FORMAT, 'level' => 'core', 'section' => '8', 'cases' => [self::CASE, ['id' => 'a/c'] + self::CASE]]);

        $this->expectExceptionMessage('The manifest states b.json as ["core",null,"8",1]; the file is ["core",null,"8",2].');

        Set::open($this->root);
    }

    public function testAFileThatIsNotJsonIsRefused(): void
    {
        $this->write('fixtures/b.json', '{"format": ');

        $this->expectExceptionMessage("{$this->root}/fixtures/b.json is not JSON this runner can read: Syntax error.");

        Set::open($this->root);
    }

    public function testADefectThisRunnerCannotReadIsRefused(): void
    {
        $this->write('defects.json', ['format' => Set::FORMAT, 'defects' => [['id' => 'raises', 'description' => 'Raises.', 'expects' => 'pass']]]);

        $this->expectExceptionMessage('defects.json holds a defect this runner cannot read: "raises".');

        Set::open($this->root, false);
    }

    /** @param array<array-key, mixed>|string $content */
    private function write(string $path, array|string $content): void
    {
        file_put_contents("{$this->root}/{$path}", is_string($content) ? $content : json_encode($content, JSON_THROW_ON_ERROR));
    }
}
