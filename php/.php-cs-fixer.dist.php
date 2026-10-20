<?php

declare(strict_types=1);

// The formatting contract: PHP's community standard, plus the family's rules
// that standard leaves open.
return (new PhpCsFixer\Config())
    ->setRiskyAllowed(true)
    ->setRules([
        '@PER-CS2.0' => true,
        'declare_strict_types' => true,
        'no_extra_blank_lines' => true,
        'no_trailing_whitespace' => true,
        'no_unused_imports' => true,
        'ordered_imports' => true,
        'single_blank_line_at_eof' => true,
        'single_quote' => true,
        'trailing_comma_in_multiline' => ['elements' => ['arrays', 'arguments', 'parameters', 'match']],
    ])
    ->setFinder(
        PhpCsFixer\Finder::create()
            ->in(__DIR__)
            ->exclude(['vendor', 'tools/qa', 'conformance/.set'])
            ->ignoreDotFiles(false)
            ->name('*.php')
            ->append([__DIR__ . '/conformance/bin/curly-message-conformance']),
    );
