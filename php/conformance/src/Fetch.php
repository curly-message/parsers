<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * Obtains the set a release of the specification ships as
 * `conformance-<version>.zip`: downloaded, held to the digest its release
 * notes publish before anything opens it, and extracted entry by entry under
 * the one directory the archive declares.
 */
final class Fetch
{
    /** The file a verified extraction leaves beside the set, naming the digest it was verified against. */
    public const STAMP = '.sha256';

    /**
     * The pin: the set's version, where its archive is published, and the
     * archive's SHA-256.
     *
     * @return array{version: string, url: string, sha256: string}
     */
    public static function pin(string $file): array
    {
        $pin = json_decode((string) file_get_contents($file), true, 4, JSON_THROW_ON_ERROR);

        if (!is_array($pin)) {
            throw new \RuntimeException("{$file} holds no pin.");
        }

        $version = $pin['version'] ?? null;
        $url = $pin['url'] ?? null;
        $sha256 = $pin['sha256'] ?? null;

        if (!is_string($version) || preg_match('/^[0-9]+\.[0-9]+\.[0-9]+$/', $version) !== 1) {
            throw new \RuntimeException("{$file} pins no version of the set.");
        }

        if (!is_string($url) || !str_starts_with($url, 'https://')) {
            throw new \RuntimeException("{$file} names no HTTPS address for the set.");
        }

        if (!is_string($sha256) || preg_match('/^[0-9a-f]{64}$/', $sha256) !== 1) {
            throw new \RuntimeException("{$file} pins no SHA-256 digest of the set.");
        }

        return ['version' => $version, 'url' => $url, 'sha256' => $sha256];
    }

    /** Where the set of a version lives once it is installed under a directory. */
    public static function root(string $into, string $version): string
    {
        return "{$into}/{$version}/conformance-{$version}";
    }

    /** Whether the set of a version is installed under a directory, verified against this digest. */
    public static function installed(string $into, string $version, string $sha256): bool
    {
        $stamp = "{$into}/{$version}/" . self::STAMP;

        return is_file($stamp) && trim((string) file_get_contents($stamp)) === $sha256 && is_dir(self::root($into, $version));
    }

    public static function download(string $url): string
    {
        $context = stream_context_create(['http' => ['follow_location' => 1, 'max_redirects' => 5, 'timeout' => 60, 'ignore_errors' => false]]);
        $archive = @file_get_contents($url, false, $context);

        if ($archive === false) {
            throw new \RuntimeException("The set could not be downloaded from {$url}: " . (error_get_last()['message'] ?? 'no reason given') . '.');
        }

        return $archive;
    }

    /**
     * Verifies an archive against its digest and extracts it. Nothing is
     * written until the digest holds, and nothing is kept unless every entry
     * lies inside the directory the archive is named for: the extraction
     * goes to a directory of its own and takes the set's place only whole.
     *
     * @return string the set's root
     */
    public static function install(string $archive, string $sha256, string $version, string $into): string
    {
        $actual = hash('sha256', $archive);

        if (!hash_equals($sha256, $actual)) {
            throw new \RuntimeException("The archive's SHA-256 is {$actual}, not the pinned {$sha256}.");
        }

        if (!is_dir($into) && !mkdir($into, 0o777, true) && !is_dir($into)) {
            throw new \RuntimeException("{$into} could not be created.");
        }

        $staging = "{$into}/.{$version}-" . bin2hex(random_bytes(6));
        $file = "{$staging}.zip";

        try {
            mkdir($staging);
            file_put_contents($file, $archive);
            self::extract($file, "conformance-{$version}/", $staging);
            file_put_contents("{$staging}/" . self::STAMP, $sha256 . "\n");
            self::remove("{$into}/{$version}");

            if (!rename($staging, "{$into}/{$version}")) {
                throw new \RuntimeException("The set could not be moved into {$into}/{$version}.");
            }
        } finally {
            if (is_file($file)) {
                unlink($file);
            }

            self::remove($staging);
        }

        return self::root($into, $version);
    }

    private static function extract(string $file, string $prefix, string $into): void
    {
        $zip = new \ZipArchive();

        if ($zip->open($file, \ZipArchive::RDONLY) !== true) {
            throw new \RuntimeException('The archive is not a zip archive.');
        }

        try {
            for ($index = 0; $index < $zip->numFiles; $index += 1) {
                $name = $zip->getNameIndex($index);

                if (!is_string($name) || !self::inside($name, $prefix)) {
                    throw new \RuntimeException('The archive holds an entry outside ' . $prefix . ': ' . json_encode($name, JSON_INVALID_UTF8_SUBSTITUTE) . '.');
                }

                $target = "{$into}/{$name}";

                if (str_ends_with($name, '/')) {
                    if (!is_dir($target)) {
                        mkdir($target, 0o777, true);
                    }

                    continue;
                }

                $content = $zip->getFromIndex($index);

                if ($content === false) {
                    throw new \RuntimeException("The archive's entry {$name} could not be read.");
                }

                if (!is_dir(dirname($target))) {
                    mkdir(dirname($target), 0o777, true);
                }

                file_put_contents($target, $content);
            }
        } finally {
            $zip->close();
        }
    }

    // An entry name is a path under the archive's one directory, spelled
    // with forward slashes and no step that leaves it.
    private static function inside(string $name, string $prefix): bool
    {
        if (!str_starts_with($name, $prefix) || strcspn($name, "\\\0:") !== strlen($name)) {
            return false;
        }

        foreach (explode('/', substr($name, strlen($prefix))) as $step) {
            if ($step === '.' || $step === '..') {
                return false;
            }
        }

        return true;
    }

    private static function remove(string $path): void
    {
        if (!file_exists($path)) {
            return;
        }

        if (!is_dir($path) || is_link($path)) {
            unlink($path);

            return;
        }

        $entries = new \RecursiveIteratorIterator(new \RecursiveDirectoryIterator($path, \FilesystemIterator::SKIP_DOTS), \RecursiveIteratorIterator::CHILD_FIRST);

        foreach ($entries as $entry) {
            if (!$entry instanceof \SplFileInfo) {
                continue;
            }

            $entry->isDir() && !$entry->isLink() ? rmdir($entry->getPathname()) : unlink($entry->getPathname());
        }

        rmdir($path);
    }
}
