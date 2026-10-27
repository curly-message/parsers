<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\ModifierInput;
use CurlyMessage\Parser;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/** Core on a host with no ext-intl: it installs, loads and resolves without it. */
final class WithoutIntlTest extends TestCase
{
    use Resolves;

    protected function setUp(): void
    {
        if (extension_loaded('intl')) {
            self::markTestSkipped('ext-intl is loaded.');
        }
    }

    /** @return iterable<string, array{string}> */
    public static function names(): iterable
    {
        foreach (['number', 'date', 'ago', 'currency', 'plural', 'ordinal'] as $name) {
            yield $name => [$name];
        }
    }

    #[DataProvider('names')]
    public function testLeavesTheIntlModifiersUnregistered(string $name): void
    {
        self::assertSame('D', $this->parser()->resolve("{{n:{$name}; one:ONE; default:D}}", ['n' => 1], locale: 'en'));
        self::assertSame(['unknown-modifier'], $this->codes());
        self::assertSame([], $this->requests);
    }

    #[DataProvider('names')]
    public function testLetsAHostRegisterItsOwnUnderTheirNames(string $name): void
    {
        $parser = $this->parser(customModifiers: [$name => static fn(ModifierInput $in): string => $name . ':' . $in->value]);

        self::assertSame($name . ':1', $parser->resolve("{{n:{$name}}}", ['n' => 1], locale: 'en'));
        self::assertSame([], $this->codes());
    }

    public function testDeclaresCoreAndExtensionsAndNothingUnexpressible(): void
    {
        self::assertSame(['core', 'extensions'], Parser::levels());
        self::assertSame([], Parser::unexpressible());
        self::assertSame('A', $this->parser()->resolve('{{v; a:A}}', ['v' => 'a']));
    }
}
