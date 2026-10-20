<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal\Intl;

use CurlyMessage\Internal\Intl\LocaleTag;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class LocaleTagTest extends TestCase
{
    /** @return iterable<string, array{string, array{string, array<string, string>}|null}> */
    public static function tags(): iterable
    {
        yield 'a language' => ['en', ['en', []]];
        yield 'any case' => ['EN-us', ['en_US', []]];
        yield 'a script and a region' => ['zh-hant-tw', ['zh_Hant_TW', []]];
        yield 'an underscore' => ['en_US', ['en_US', []]];
        yield 'a numeric region' => ['es-419', ['es_419', []]];
        yield 'variants' => ['sl-rozaj-biske', ['sl__ROZAJ_BISKE', []]];
        yield 'keywords, the first of each' => ['en-US-u-nu-arab-ca-gregory-nu-thai', ['en_US', ['nu' => 'arab', 'ca' => 'gregory']]];
        yield 'a transformed extension' => ['en-t-de-h0-hybrid-u-nu-latn', ['en', ['nu' => 'latn']]];
        yield 'private use' => ['en-x-foo', ['en', []]];
        yield 'root' => ['root', ['root', []]];
        yield 'not a tag' => ['not a locale', null];
        yield 'a character outside one' => ['x!!', null];
        yield 'private use alone' => ['x-foo', null];
        yield 'an ICU keyword' => ['de@collation=phonebook', null];
        yield 'a grandfathered tag' => ['i-klingon', null];
        yield 'a singleton twice' => ['en-a-x-a-y', null];
        yield 'a variant twice' => ['de-1996-1996', null];
        yield 'an empty subtag' => ['de--DE', null];
        yield 'a trailing separator' => ['en-', null];
        yield 'a leading one' => ['-en', null];
        yield 'an extension with nothing in it' => ['en-u', null];
        yield 'a subtag too long' => ['abcdefghi', null];
        yield 'nothing' => ['', null];
    }

    /** @param array{string, array<string, string>}|null $parsed */
    #[DataProvider('tags')]
    public function testReadsAUnicodeLocaleIdentifierAndNothingElse(string $tag, ?array $parsed): void
    {
        self::assertSame($parsed, LocaleTag::parse($tag));
    }
}
