<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

use CurlyMessage\Parser;
use CurlyMessage\Undefined;

/**
 * The text a value resolves to (SPEC.md section 4), for one resolution.
 * Everything the format carries is text: a PHP array and a `stdClass` are
 * plain, PHP's types for arbitrary keyed data and sequences, and become JSON,
 * so a modifier can read them back; every other value becomes what PHP makes
 * of it as a string.
 *
 * Converting can run host code (`__toString`, `jsonSerialize`, and on 8.4 a
 * property hook or a lazy initializer), so a value is converted once in a
 * resolution, the answer that nothing describes it included: an object by
 * its identity, and an array, which has none, by where the resolution read
 * it, beside the array, so that one host code put there since is a new value
 * and converts, as far as the resolution compares what it reads again
 * (`same`); one handed over with no place converts unrecorded. A scalar runs
 * no host code and converts alike every time, so it is not recorded. A
 * conversion at a place is recorded as begun before any host code runs, and a
 * read of the value before it finished, from inside it or from a fiber while
 * the one converting it is suspended, converts nothing and answers that
 * nothing describes the value: one that converted again would, where each of
 * a modifier's fibers reads it, convert it once a fiber, and a conversion a
 * fiber was let go in the middle of never finishes. What is recorded of
 * arrays is let go as the resolution returns (`release`), after which
 * nothing is read on its account: a conversion a fiber resumes then calls no
 * more host code and records nothing.
 *
 * @internal
 */
final class Conversion
{
    private const FLAGS = JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_LINE_TERMINATORS;

    /** Bytes of a long string escaped at once. */
    private const PIECE = 65_536;

    /**
     * Containers nested on the deepest path of an array `===` is asked to
     * compare: it walks one C frame per level, on the stack of the fiber
     * that asks, a stack of 2 MiB by default, which a host may make smaller.
     */
    private const SHALLOW = 1_000;

    /** @var \WeakMap<object, string|bool> false where nothing describes the object, true while it converts */
    private \WeakMap $objects;

    /** @var array<string, array{array<array-key, mixed>, string|false, bool, bool}|true> each place's array beside its text, false where nothing describes it, whether `===` may be asked of it, and whether its text holds a float zero; true while it converts */
    private array $places = [];

    /** Whether the resolution returned and let go of what was recorded. */
    private bool $released = false;

    /** Entries the resolution's re-reads compared one by one, together. */
    private int $compared = 0;

    /** @var \WeakMap<object, true> objects that hold more properties than a conversion visits */
    private \WeakMap $wide;

    /** @var array<class-string, bool> whether a class declares hooked properties */
    private static array $hooked = [];

    /** @var (\Closure(object): array<mixed>)|null reads the public properties, from no class's scope */
    private static ?\Closure $publicProperties = null;

    /** @param int $length the bytes a text holds at most; a test writes to a shorter one */
    public function __construct(private readonly int $length = Parser::MAX_TEXT_LENGTH)
    {
        $this->objects = new \WeakMap();
        $this->wide = new \WeakMap();
    }

    /**
     * The value's text, or null where there is none: for `Undefined::Value`,
     * which is no value, and for a value nothing describes, which the caller
     * tells apart and reports.
     *
     * @param string|null $place where the resolution read an array, or null for one converted unrecorded
     */
    public function text(mixed $value, ?string $place = null): ?string
    {
        if (is_string($value)) {
            return $value;
        }

        if (is_object($value)) {
            $text = $this->objects[$value] ?? null;

            if ($text === null) {
                $this->objects[$value] = true;
                $text = $this->objects[$value] = $this->object($value) ?? false;
            }

            return is_string($text) ? $text : null;
        }

        if (is_array($value)) {
            if ($place === null) {
                return $this->serialize($value);
            }

            $record = $this->places[$place] ?? null;

            if ($record === true) {
                return null;
            }

            if ($record === null || !$this->same($record, $value)) {
                $this->places[$place] = true;
                $deepest = 0;
                $zero = false;
                $text = $this->serialize($value, $deepest, $zero) ?? false;
                $record = [$value, $text, $text !== false && $deepest <= self::SHALLOW, $zero];

                // A fiber left suspended in the conversion may finish it once
                // the resolution let go of its records, which it must not
                // hold again.
                if (!$this->released) {
                    $this->places[$place] = $record;
                }
            }

            return is_string($record[1]) ? $record[1] : null;
        }

        if ($value === null) {
            return '';
        }

        if (is_bool($value) || is_int($value) || is_float($value)) {
            return (string) $value;
        }

        // A resource, open or closed, is the one kind left.
        return is_resource($value) || gettype($value) === 'resource (closed)' ? 'Resource id #' . get_resource_id($value) : null;
    }

    /**
     * Lets go of what the resolution recorded, where nothing a `__destruct`
     * runs leaves it, as the resolution returns.
     */
    public function release(): void
    {
        $this->released = true;

        if ($this->places === []) {
            return;
        }

        $held = $this->places;
        $this->places = [];
        Contained::release($held);
    }

    /**
     * Whether an array read again at a place is the one its record holds:
     * entries that convert alike. `===` tells, which costs nothing where it
     * is the same array, and otherwise stops at the first entry that
     * differs. Over an array that converted it walks no further than the
     * conversion did, but it walks a C frame for each level, so a record
     * nested deeper than `SHALLOW` is compared entry by entry on a stack of
     * the walk's own. One nothing describes may share more than a conversion
     * visits, which `===` would walk each time it is reached, so it is
     * compared entry by entry too. `===` tells a float zero alike to one of
     * either sign, which the text spells, so a record whose text holds one
     * that no object holds is compared entry by entry once `===` tells them
     * alike (`signed`).
     *
     * @param array{array<array-key, mixed>, string|false, bool, bool} $record
     * @param array<array-key, mixed> $read
     */
    private function same(array $record, array $read): bool
    {
        if ($record[2]) {
            return $record[0] === $read && (!$record[3] || $this->signed($record[0], $read));
        }

        return $this->compare($record[0], $read);
    }

    /**
     * Whether `$read` holds entries that convert alike to those of
     * `$record`, compared entry by entry (`alike`). What the resolution's
     * comparisons walk is held, together, to what one conversion visits:
     * past it, an array holding as many entries as the record is taken for
     * it, so reading such an array at every placeholder of a message costs
     * no more than converting it once.
     *
     * @param array<array-key, mixed> $record
     * @param array<array-key, mixed> $read
     */
    private function compare(array $record, array $read): bool
    {
        $pairs = [[$record, $read]];

        while (($pair = array_pop($pairs)) !== null) {
            [$recorded, $now] = $pair;
            $count = count($recorded);

            if ($count !== count($now)) {
                return false;
            }

            $this->compared += $count;

            if ($this->compared > Parser::MAX_CONVERSION_NODES) {
                return true;
            }

            if (array_keys($recorded) !== array_keys($now)) {
                return false;
            }

            foreach ($recorded as $key => $entry) {
                $other = $now[$key];

                if (is_array($entry) && is_array($other)) {
                    $pairs[] = [$entry, $other];
                } elseif (!self::alike($entry, $other)) {
                    return false;
                }
            }
        }

        return true;
    }

    /**
     * Whether each float zero of `$record`, an array `===` tells alike to
     * `$read`, is of the sign of the one `$read` holds in its place, where
     * `===` tells them alike whatever their signs. What it walks counts
     * against what the comparisons may walk together (`compare`): past it,
     * the two are taken for alike, so all a comparison past it can miss is
     * the sign of a zero.
     *
     * @param array<array-key, mixed> $record
     * @param array<array-key, mixed> $read
     */
    private function signed(array $record, array $read): bool
    {
        $pairs = [[$record, $read]];

        while (($pair = array_pop($pairs)) !== null) {
            [$recorded, $now] = $pair;
            $this->compared += count($recorded);

            if ($this->compared > Parser::MAX_CONVERSION_NODES) {
                return true;
            }

            foreach ($recorded as $key => $entry) {
                $other = $now[$key];

                if (is_array($entry) && is_array($other)) {
                    $pairs[] = [$entry, $other];
                } elseif ($entry === 0.0 && is_float($other) && fdiv(1, $entry) !== fdiv(1, $other)) {
                    return false;
                }
            }
        }

        return true;
    }

    /**
     * Whether two entries that are no arrays convert alike: as `===` tells,
     * but for a float that is not a number, alike to itself, as the same
     * array's entry is, and a float zero, alike only to one of its sign.
     */
    private static function alike(mixed $recorded, mixed $now): bool
    {
        if (!is_float($recorded) || !is_float($now)) {
            return $recorded === $now;
        }

        if (is_nan($recorded)) {
            return is_nan($now);
        }

        return $recorded === $now && ($recorded !== 0.0 || fdiv(1, $recorded) === fdiv(1, $now));
    }

    private function object(object $value): ?string
    {
        if ($value === Undefined::Value) {
            return null;
        }

        if (get_class($value) === \stdClass::class) {
            return $this->serialize($value);
        }

        // What PHP makes of an object as a string: its `__toString`, or the
        // number a GMP is. An object with neither has no text, which is the
        // `Error` the cast would raise. The text is made by the time it is
        // answered, and keeping it copies nothing, so it is the object's text
        // however long.
        return Contained::run(static function () use ($value): ?string {
            if ($value instanceof \Stringable) {
                return $value->__toString();
            }

            return $value instanceof \GMP ? (string) $value : null;
        }, null);
    }

    /**
     * JSON as `json_encode` writes it with `FLAGS`, byte for byte, over every
     * shape it accepts, but written here: `json_encode` neither counts what
     * it visits nor stops, and it would call `jsonSerialize` past any bound.
     * Every value visited counts against `MAX_CONVERSION_NODES`, so a value
     * sharing one child at every level, which visits that child again each
     * time, is refused rather than written out at a cost growing
     * exponentially with what it holds. And the text stops at
     * `Parser::MAX_TEXT_LENGTH`, so a value sharing one long string or one
     * long key many times is refused rather than written out past it, as
     * the JavaScript parser's engine refuses a string past its longest,
     * though that one counts UTF-16 units and this bytes. The walk runs
     * contained, so what a `jsonSerialize` answered, which it holds, is let
     * go inside the containment too.
     *
     * @param array<array-key, mixed>|\stdClass $root
     * @param int $deepest set to the most containers the walk had open at once
     * @param bool $zero set where the walk wrote a float zero no object holds, whose sign the text spells and `===` does not tell
     */
    private function serialize(array|\stdClass $root, int &$deepest = 0, bool &$zero = false): ?string
    {
        return Contained::run(function () use ($root, &$deepest, &$zero): ?string {
            return $this->write($root, $deepest, $zero);
        }, null);
    }

    /**
     * The walk is a loop over an explicit stack of the containers open on the
     * path to the value being written, each with where it is in its members,
     * so no shape nests a call per level: a map's keys are listed as it
     * opens, a key is written from the payload's own string, and a long
     * string is escaped a piece at a time. The text is held to its length
     * exactly, and the containers open on the path, each a value visited,
     * to the conversion limit.
     *
     * @param array<array-key, mixed>|\stdClass $root
     */
    private function write(array|\stdClass $root, int &$deepest, bool &$zero): ?string
    {
        $value = $root;
        $visits = 1;
        $out = '';
        /** @var array<int, object> $path the objects being written, which none of them may hold again, held so that no other object takes an id on it */
        $path = [];
        /** @var list<array{0: array<array-key, mixed>, 1: list<array-key>|null, 2: int, 3: int, 4: bool, 5: list<int>}> $open each container open: its members, its keys where it is a map, how many it has, the next to write, whether it wrote one, and the objects it releases */
        $open = [];

        while (true) {
            if (strlen($out) > $this->length) {
                return null;
            }

            $held = [];
            $asked = false;

            // An answer that is again serializable is asked in turn; one that
            // is the object itself is written as its properties. The first
            // answer stands in for the value, as ECMAScript's `toJSON` answer
            // does, and each one asked after it is a value visited, so a
            // chain of new answers ends at the limit.
            while ($value instanceof \JsonSerializable) {
                if ($asked && ++$visits > Parser::MAX_CONVERSION_NODES || $this->released) {
                    return null;
                }

                $id = spl_object_id($value);

                if (isset($path[$id])) {
                    return null;
                }

                $path[$id] = $value;
                $held[] = $id;
                $answer = $value->jsonSerialize();
                $asked = true;

                if ($answer === $value) {
                    break;
                }

                $value = $answer;
            }

            unset($answer);

            // A fiber left suspended in host code the conversion called may
            // resume it once the resolution returned, after which it calls
            // no more.
            if (is_object($value) && $this->released) {
                return null;
            }

            $members = is_array($value) || is_object($value) ? $this->members($value, $visits, $path, $held) : null;

            if ($members === false) {
                return null;
            }

            if ($members === null) {
                $this->scalar($value, $out);
                // A zero an object holds is compared as the object is, by
                // its identity.
                $zero = $zero || $value === 0.0 && $path === [];

                foreach ($held as $id) {
                    unset($path[$id]);
                }
            } else {
                [$isList, $entries] = $members;
                $visits += count($entries);

                if ($visits > Parser::MAX_CONVERSION_NODES) {
                    if (is_object($value) && count($entries) >= Parser::MAX_CONVERSION_NODES) {
                        $this->wide[$value] = true;
                    }

                    return null;
                }

                $out .= $isList ? '[' : '{';
                $open[] = [$entries, $isList ? null : array_keys($entries), count($entries), 0, false, $held];
                $deepest = max($deepest, count($open));
            }

            $value = null;

            // The next member to write, closing each container that has none
            // left. ECMAScript leaves its undefined out of an object and
            // writes it as null in an array.
            while (true) {
                $top = array_key_last($open);

                if ($top === null) {
                    return strlen($out) <= $this->length ? $out : null;
                }

                [$entries, $keys, $count, $next, $wrote] = $open[$top];

                if ($keys !== null) {
                    while ($next < $count && $entries[$keys[$next]] === Undefined::Value) {
                        $next += 1;
                    }
                }

                if ($next < $count) {
                    break;
                }

                $out .= $keys === null ? ']' : '}';

                foreach ($open[$top][5] as $id) {
                    unset($path[$id]);
                }

                array_pop($open);
            }

            if ($wrote) {
                $out .= ',';
            }

            $open[$top] = [$entries, $keys, $count, $next + 1, true, $open[$top][5]];

            if ($keys === null) {
                $value = $entries[$next];
            } else {
                $key = $keys[$next];
                $this->string((string) $key, $out);
                $out .= ':';
                $value = $entries[$key];
            }

            unset($entries, $keys);
        }
    }

    /**
     * What a container holds, and whether it is written as a list, null for
     * a value written whole, or false for a container the conversion may not
     * list. Listing what a container holds walks every member of it, so an
     * array is counted before it is listed, and an object listed past what
     * any conversion visits is refused wherever the resolution meets it
     * again: no conversion costs more than the values it may visit, however
     * often the resolution meets one too wide.
     *
     * @param array<array-key, mixed>|object $value
     * @param int $visits the values the conversion visited so far
     * @param array<int, object> $path
     * @param list<int> $held
     *
     * @return array{bool, array<array-key, mixed>}|false|null
     */
    private function members(array|object $value, int $visits, array &$path, array &$held): array|false|null
    {
        if (is_array($value)) {
            return $visits + count($value) > Parser::MAX_CONVERSION_NODES ? false : [array_is_list($value), $value];
        }

        if ($value instanceof \UnitEnum) {
            return null;
        }

        if (isset($this->wide[$value])) {
            return false;
        }

        $id = spl_object_id($value);

        if (!in_array($id, $held, true)) {
            if (isset($path[$id])) {
                throw new \JsonException('Recursion detected');
            }

            $path[$id] = $value;
            $held[] = $id;
        }

        return [false, $value instanceof \Closure ? [] : self::properties($value)];
    }

    /**
     * An object's properties as `json_encode` lists them: what the `(array)`
     * cast reads, which includes the table an internal class supplies for
     * JSON, less every private and protected name. On 8.4 `json_encode`
     * initializes a lazy object and reads a hooked property through its hook,
     * and so does this.
     *
     * @return array<array-key, mixed>
     */
    private static function properties(object $value): array
    {
        $class = new \ReflectionClass($value);

        // Lazy objects exist from 8.4 on, which the methods asked for mark.
        if (method_exists($class, 'isUninitializedLazyObject') && method_exists($class, 'initializeLazyObject') && $class->isUninitializedLazyObject($value)) {
            $class->initializeLazyObject($value);
        }

        if (self::$hooked[$class->getName()] ??= self::declaresHooks($class)) {
            self::$publicProperties ??= \Closure::bind(static fn(object $object): array => get_object_vars($object), null, null);

            return (self::$publicProperties)($value);
        }

        $properties = (array) $value;

        foreach ($properties as $name => $property) {
            if (is_string($name) && str_starts_with($name, "\0")) {
                unset($properties[$name]);
            }
        }

        return $properties;
    }

    /** @param \ReflectionClass<object> $class */
    private static function declaresHooks(\ReflectionClass $class): bool
    {
        foreach ($class->getProperties() as $property) {
            // Hooks exist from 8.4 on, which the method asked for marks.
            if (method_exists($property, 'hasHooks') && $property->hasHooks()) {
                return true;
            }
        }

        return false;
    }

    /** Writes a value that is no container. */
    private function scalar(mixed $value, string &$out): void
    {
        if ($value === Undefined::Value) {
            $out .= 'null';

            return;
        }

        if (is_string($value)) {
            $this->string($value, $out);

            return;
        }

        if ($value instanceof \BackedEnum) {
            $value = $value->value;
        } elseif ($value instanceof \UnitEnum) {
            throw new \JsonException('Non-backed enums have no default serialization');
        }

        $out .= json_encode($value, self::FLAGS | JSON_THROW_ON_ERROR);
    }

    /**
     * Writes a string as JSON. Escaping only lengthens it, so one the text
     * has no room for is refused before anything of it is copied; a long one
     * is escaped a piece at a time, each cut before a byte that begins a
     * character, so what escaping makes of it never stands beside the text
     * whole. A piece cut so never splits a well-formed character, and a
     * string that is not UTF-8 is refused in whichever piece holds the fault.
     */
    private function string(string $value, string &$out): void
    {
        $size = strlen($value);

        if ($size + 2 > $this->length - strlen($out)) {
            throw new \LengthException('The text passes its limit');
        }

        if ($size <= self::PIECE) {
            $out .= json_encode($value, self::FLAGS | JSON_THROW_ON_ERROR);

            return;
        }

        $out .= '"';

        for ($at = 0; $at < $size; $at = $cut) {
            $cut = $at + self::PIECE;

            if ($cut >= $size) {
                $cut = $size;
            } else {
                // A character spans at most three bytes after its first.
                for ($back = 0; $back < 3 && (ord($value[$cut]) & 0xC0) === 0x80; $back += 1) {
                    $cut -= 1;
                }
            }

            $out .= substr(json_encode(substr($value, $at, $cut - $at), self::FLAGS | JSON_THROW_ON_ERROR), 1, -1);

            if (strlen($out) > $this->length) {
                throw new \LengthException('The text passes its limit');
            }
        }

        $out .= '"';
    }
}
