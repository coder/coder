// scripts/check-sentence-case.js
//
// Checks for title case text that should use sentence case.
// Run from the site/ directory: node ../scripts/check-sentence-case/check-sentence-case.js

import fs from "node:fs";
import path from "node:path";

// Files excluded from checking:
// - *Generated.ts (auto-generated files)
// - *.stories.tsx, *.stories.ts (Storybook - often has intentional title case)
// - *.test.ts, *.test.tsx, *.jest.ts, *.jest.tsx (test files)
// - mocks.ts, mocks.tsx, storybookUtils.ts (mock/test data files)
// - testHelpers/** (test helper utilities)

// Proper nouns that should remain capitalized.
// - Multi-word entries: skip detection entirely for exact matches
// - Single-word entries: preserve capitalization during conversion
const PROPER_NOUNS = new Set([
  // === Single words (preserved during conversion) ===
  // Acronyms and initialisms
  "API",
  "CLI",
  "CSS",
  "DNS",
  "HTML",
  "HTTP",
  "HTTPS",
  "ID",
  "IP",
  "JSON",
  "JWT",
  "OAuth",
  "OIDC",
  "SSH",
  "SSL",
  "TCP",
  "TLS",
  "UDP",
  "UI",
  "URI",
  "URL",
  "VPN",
  "VSCode",
  "WebSocket",

  // Product/brand names (single words)
  "Coder",
  "Git",
  "GitHub",
  "Github", // Common misspelling, keep capitalized
  "Terraform",

  // === Multi-word phrases (skip detection entirely) ===
  // Products and services
  "Visual Studio Code",
  "Amazon Web Services",
  "Google Cloud Platform",
  "Google Cloud",
  "Microsoft Azure",
  "Open Source",
  "Coder Desktop",
  "Code Desktop",
  "Code Insiders",
  "Dev Container",
  "Git Clone",
	"VS Code Desktop",
	"VS Code Insiders",

  // Geographic names (countries, territories, regions)
  "British Indian Ocean Territory",
  "Central African Republic",
  "French Southern Territories",
  "Northern Mariana Islands",
  "Papua New Guinea",
  "South Sandwich Islands",
  "Syrian Arab Republic",
  "United Arab Emirates",
  "United States Minor Outlying Islands",
  "New York City",
  "Former Yugoslav Republic",

  // Font names
  "Source Code Pro",
  "Lucida Sans Typewriter",
  "Lucida Console",
  "Liberation Mono",
  "Courier New",
  "Inter Variable",
  "IBM Plex Mono",
  "Plex Mono",
  "Fira Code",
  "JetBrains Mono",

  // Technical terms
  "Web Terminal Font",
  "Single Sign On",
	"OIDC Group Mapping",

]);

// Words that should NOT be capitalized in title case (unless first word).
const MINOR_WORDS = new Set([
  "a",
  "an",
  "the", // articles
  "and",
  "but",
  "or",
  "nor",
  "for",
  "yet",
  "so", // conjunctions
  "at",
  "by",
  "for",
  "in",
  "of",
  "on",
  "to",
  "up", // prepositions
  "as",
  "if",
  "or",
  "vs",
  "via",
]);

// Pattern: 2+ consecutive capitalized words.
// Matches: "Title Case", "ACRONYM Title Case", "Title ACRONYM Case"
// Word types: [A-Z][a-z]+ (Title) or [A-Z]{2,} (ACRONYM)
const TITLE_CASE_PATTERN =
  /(?:^|>|\s)((?:[A-Z]{2,}\s+)?[A-Z][a-z]+(?:\s+(?:[A-Z][a-z]+|[A-Z]{2,}))+)(?:\s|<|$|[.!?])/g;

// Inline ignore comment pattern.
const IGNORE_COMMENT = "sentence-case-ignore";

/**
 * Convert a title case phrase to sentence case.
 * Keeps the first word capitalized, lowercases subsequent words
 * (unless they're acronyms or known proper nouns).
 */
function toSentenceCase(phrase) {
  const words = phrase.split(/\s+/);

  return words
    .map((word, index) => {
      // Keep first word as-is (whether it's an acronym or regular word).
      if (index === 0) {
        return word;
      }

      // Keep acronyms (all caps, 2+ letters) as-is.
      if (isAcronym(word)) {
        return word;
      }

      // Check if word should stay capitalized (known proper nouns).
      if (PROPER_NOUNS.has(word)) {
        return word;
      }

      // Lowercase the word.
      return word.toLowerCase();
    })
    .join(" ");
}

function isAcronym(word) {
  return /^[A-Z]{2,}$/.test(word);
}

function isTitleCase(phrase) {
  const words = phrase.trim().split(/\s+/);
  if (words.length < 2) return false;

  // Check if all words are capitalized (either Title case or ACRONYM).
  const allCapped = words.every((w) => /^[A-Z]/.test(w));
  if (!allCapped) return false;

  // Check if any non-first word is a minor word that's capitalized.
  for (let i = 1; i < words.length; i++) {
    const lower = words[i].toLowerCase();
    if (MINOR_WORDS.has(lower)) {
      return true;
    }
  }

  // Flag if 2+ regular words (non-acronyms, non-minor) are capitalized.
  // Or if there's an acronym followed by a capitalized word.
  const titleCaseWords = words.filter(
    (w) => !MINOR_WORDS.has(w.toLowerCase()) && !isAcronym(w),
  );
  return titleCaseWords.length >= 2 || (isAcronym(words[0]) && titleCaseWords.length >= 1);
}

function checkFile(filePath, fix = false) {
  const content = fs.readFileSync(filePath, "utf8");
  const issues = [];
  const lines = content.split("\n");
  let modified = false;

  const newLines = lines.map((line, idx) => {
    // Skip lines with ignore comment.
    if (line.includes(IGNORE_COMMENT)) {
      return line;
    }

    let newLine = line;
    const matches = line.matchAll(/["'`>]([^"'`<]+)["'`<]/g);

    for (const match of matches) {
      const text = match[1];
      const phraseMatches = text.matchAll(TITLE_CASE_PATTERN);

      for (const phraseMatch of phraseMatches) {
        const phrase = phraseMatch[1];

        // Skip phrases that are proper nouns.
        if (PROPER_NOUNS.has(phrase)) {
          continue;
        }

        if (isTitleCase(phrase)) {
          const sentenceCase = toSentenceCase(phrase);

          // Skip if the fix doesn't change anything (already correct or all proper nouns).
          if (sentenceCase === phrase) {
            continue;
          }

          issues.push({
            line: idx + 1,
            text: phrase,
            fixed: sentenceCase,
            file: filePath,
          });

          if (fix) {
            newLine = newLine.replace(phrase, sentenceCase);
            modified = true;
          }
        }
      }
    }

    return newLine;
  });

  if (fix && modified) {
    fs.writeFileSync(filePath, newLines.join("\n"), "utf8");
  }

  return issues;
}

/**
 * Recursively find all files matching the given extensions.
 */
function findFiles(dir, extensions) {
  const results = [];

  function walk(currentDir) {
    const entries = fs.readdirSync(currentDir, { withFileTypes: true });

    for (const entry of entries) {
      const fullPath = path.join(currentDir, entry.name);

      if (entry.isDirectory()) {
        // Skip node_modules and hidden directories.
        if (entry.name !== "node_modules" && !entry.name.startsWith(".")) {
          walk(fullPath);
        }
      } else if (entry.isFile()) {
        const ext = path.extname(entry.name);
        if (extensions.includes(ext)) {
          results.push(fullPath);
        }
      }
    }
  }

  walk(dir);
  return results;
}

/**
 * Check if a file path matches any of the exclusion patterns.
 */
function shouldExclude(filePath) {
  const normalizedPath = filePath.replace(/\\/g, "/");
  const fileName = path.basename(filePath);

  // Check file name patterns.
  if (fileName.endsWith("Generated.ts")) return true;
  if (fileName.endsWith(".stories.tsx")) return true;
  if (fileName.endsWith(".stories.ts")) return true;
  if (fileName.endsWith(".test.ts")) return true;
  if (fileName.endsWith(".test.tsx")) return true;
  if (fileName.endsWith(".jest.ts")) return true;
  if (fileName.endsWith(".jest.tsx")) return true;
	if (fileName.endsWith("storybookData.ts")) return true;
  if (fileName === "mocks.ts" || fileName === "mocks.tsx") return true;
  if (fileName === "storybookUtils.ts") return true;

  // Check path patterns.
  if (normalizedPath.includes("/testHelpers/")) return true;
  if (normalizedPath.includes("/storybookData/")) return true;

  return false;
}

function main() {
  const args = process.argv.slice(2);
  const fix = args.includes("--fix");

  // Find all tsx/ts files.
  const allFiles = findFiles("src", [".ts", ".tsx"]);

  // Filter out excluded files.
  const files = allFiles.filter((file) => !shouldExclude(file));

  let allIssues = [];
  for (const file of files) {
    allIssues = allIssues.concat(checkFile(file, fix));
  }

  if (allIssues.length > 0) {
    if (fix) {
      console.info(
        `\x1b[32m✓ Fixed ${allIssues.length} sentence case issue(s):\x1b[0m\n`,
      );
      for (const { file, line, text, fixed } of allIssues) {
        console.info(`  ${file}:${line}`);
        console.info(`    "${text}" → "${fixed}"\n`);
      }
    } else {
      console.error(
        "\x1b[31mFound Title Case text (should use sentence case):\x1b[0m\n",
      );
      for (const { file, line, text, fixed } of allIssues) {
        console.error(`  ${file}:${line}`);
        console.error(`    "${text}" → "${fixed}"\n`);
      }
      console.error(`\n${allIssues.length} issue(s) found.`);
      console.error("\nTo auto-fix: pnpm run lint:sentence-case --fix");
      console.error(
        "To ignore: Add a comment containing 'sentence-case-ignore' on the same line.",
      );
      console.error(
        "For proper nouns: Add to PROPER_NOUNS in scripts/check-sentence-case/check-sentence-case.js\n",
      );
      process.exit(1);
    }
  } else {
		console.info(
			"\x1b[32m✓ No sentence case issues found.\x1b[0m\n",
		);
	}
}

main();
