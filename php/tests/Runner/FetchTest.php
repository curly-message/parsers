<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\Fetch;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class FetchTest extends TestCase
{
    private string $home;

    protected function setUp(): void
    {
        $this->home = sys_get_temp_dir() . '/curly-fetch-' . bin2hex(random_bytes(6));
        mkdir($this->home);
    }

    protected function tearDown(): void
    {
        self::remove($this->home);
    }

    public function testAVerifiedArchiveIsInstalledWithItsStamp(): void
    {
        $archive = self::zip(['conformance-1.2.3/' => null, 'conformance-1.2.3/index.json' => '{}', 'conformance-1.2.3/fixtures/a.json' => '[]']);
        $sha256 = hash('sha256', $archive);
        $into = "{$this->home}/.set";

        self::assertFalse(Fetch::installed($into, '1.2.3', $sha256));
        self::assertSame("{$into}/1.2.3/conformance-1.2.3", Fetch::install($archive, $sha256, '1.2.3', $into));
        self::assertSame('[]', file_get_contents("{$into}/1.2.3/conformance-1.2.3/fixtures/a.json"));
        self::assertSame("{$sha256}\n", file_get_contents("{$into}/1.2.3/" . Fetch::STAMP));
        self::assertTrue(Fetch::installed($into, '1.2.3', $sha256));
        self::assertFalse(Fetch::installed($into, '1.2.3', str_repeat('0', 64)));
        self::assertFalse(Fetch::installed($into, '1.2.4', $sha256));
        self::assertSame(['1.2.3'], self::listing($into));
    }

    public function testAnArchiveThatIsNotThePinnedOneWritesNothing(): void
    {
        $archive = self::zip(['conformance-1.2.3/index.json' => '{}']);
        $into = "{$this->home}/.set";

        try {
            Fetch::install($archive, str_repeat('0', 64), '1.2.3', $into);
            self::fail('The install went ahead.');
        } catch (\RuntimeException $error) {
            self::assertSame("The archive's SHA-256 is " . hash('sha256', $archive) . ', not the pinned ' . str_repeat('0', 64) . '.', $error->getMessage());
        }

        self::assertFileDoesNotExist($into);
    }

    /** @return iterable<string, array{string}> */
    public static function outside(): iterable
    {
        yield 'another directory' => ['other/index.json'];
        yield 'a directory of a longer name' => ['conformance-1.2.30/index.json'];
        yield 'a step out' => ['conformance-1.2.3/../index.json'];
        yield 'a step out further in' => ['conformance-1.2.3/fixtures/../../index.json'];
        yield 'a step that stays' => ['conformance-1.2.3/./index.json'];
        yield 'a backslash' => ['conformance-1.2.3/..\\index.json'];
        yield 'a drive' => ['conformance-1.2.3/C:index.json'];
    }

    #[DataProvider('outside')]
    public function testAnEntryOutsideTheArchivesDirectoryKeepsNothing(string $entry): void
    {
        $archive = self::zip(['conformance-1.2.3/index.json' => '{}', $entry => 'x']);
        $into = "{$this->home}/.set";

        try {
            Fetch::install($archive, hash('sha256', $archive), '1.2.3', $into);
            self::fail('The install went ahead.');
        } catch (\RuntimeException $error) {
            self::assertSame('The archive holds an entry outside conformance-1.2.3/: ' . json_encode($entry) . '.', $error->getMessage());
        }

        self::assertSame([], self::listing($into));
        self::assertFileDoesNotExist("{$this->home}/index.json");
    }

    public function testAReinstallReplacesTheSetWhole(): void
    {
        $into = "{$this->home}/.set";
        $first = self::zip(['conformance-1.2.3/a.json' => '1']);
        $second = self::zip(['conformance-1.2.3/b.json' => '2']);

        Fetch::install($first, hash('sha256', $first), '1.2.3', $into);
        Fetch::install($second, hash('sha256', $second), '1.2.3', $into);

        self::assertFileDoesNotExist("{$into}/1.2.3/conformance-1.2.3/a.json");
        self::assertSame('2', file_get_contents("{$into}/1.2.3/conformance-1.2.3/b.json"));
        self::assertTrue(Fetch::installed($into, '1.2.3', hash('sha256', $second)));
        self::assertFalse(Fetch::installed($into, '1.2.3', hash('sha256', $first)));
        self::assertSame(['1.2.3'], self::listing($into));
    }

    public function testAnArchiveThatIsNoZipIsRefused(): void
    {
        $this->expectExceptionMessage('The archive is not a zip archive.');

        Fetch::install('not a zip', hash('sha256', 'not a zip'), '1.2.3', "{$this->home}/.set");
    }

    public function testThePinOfTheRunnerIsAVersionAnAddressAndADigest(): void
    {
        $pin = Fetch::pin(dirname(__DIR__, 2) . '/conformance/set.json');

        self::assertSame("https://github.com/curly-message/spec/releases/download/conformance-v{$pin['version']}/conformance-{$pin['version']}.zip", $pin['url']);
        self::assertMatchesRegularExpression('/^[0-9a-f]{64}$/', $pin['sha256']);
    }

    /** @return iterable<string, array{array<string, string>, string}> */
    public static function pins(): iterable
    {
        $good = ['version' => '1.2.3', 'url' => 'https://example.test/set.zip', 'sha256' => str_repeat('a', 64)];

        yield 'no version' => [['version' => 'v1'] + $good, 'pins no version of the set.'];
        yield 'an address not HTTPS' => [['url' => 'http://example.test/set.zip'] + $good, 'names no HTTPS address for the set.'];
        yield 'a digest in capitals' => [['sha256' => str_repeat('A', 64)] + $good, 'pins no SHA-256 digest of the set.'];
    }

    /** @param array<string, string> $pin */
    #[DataProvider('pins')]
    public function testAPinThatDoesNotNameTheSetIsRefused(array $pin, string $message): void
    {
        $file = "{$this->home}/set.json";
        file_put_contents($file, json_encode($pin, JSON_THROW_ON_ERROR));

        $this->expectExceptionMessage("{$file} {$message}");

        Fetch::pin($file);
    }

    /** @param array<string, string|null> $entries a null entry is a directory */
    private static function zip(array $entries): string
    {
        $file = sys_get_temp_dir() . '/curly-archive-' . bin2hex(random_bytes(6)) . '.zip';
        $zip = new \ZipArchive();
        self::assertTrue($zip->open($file, \ZipArchive::CREATE | \ZipArchive::EXCL));

        foreach ($entries as $name => $content) {
            self::assertTrue($content === null ? $zip->addEmptyDir($name) : $zip->addFromString($name, $content));
        }

        self::assertTrue($zip->close());
        $archive = (string) file_get_contents($file);
        unlink($file);

        return $archive;
    }

    /** @return list<string> */
    private static function listing(string $directory): array
    {
        $entries = scandir($directory);
        self::assertIsArray($entries);

        return array_values(array_diff($entries, ['.', '..']));
    }

    private static function remove(string $path): void
    {
        if (!file_exists($path)) {
            return;
        }

        if (is_dir($path) && !is_link($path)) {
            foreach (self::listing($path) as $entry) {
                self::remove("{$path}/{$entry}");
            }

            rmdir($path);

            return;
        }

        unlink($path);
    }
}
