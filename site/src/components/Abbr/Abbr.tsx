import { type FC, type HTMLAttributes } from "react";

type AbbrProps = HTMLAttributes<HTMLElement> & {
  children: string;
  title: string;
  pronunciation?: "shorthand" | "acronym" | "initialism";
};

/**
 * A more sophisticated version of the native <abbr> element.
 *
 * Features:
 * - Better type-safety (requiring you to include certain properties)
 * - All built-in HTML styling is stripped away by default
 * - Better integration with screen readers (like exposing the title prop to
 *   them), with more options for influencing how they pronounce text
 */
export const Abbr: FC<AbbrProps> = ({
  children,
  title: visualTitle,
  pronunciation = "shorthand",
  ...delegatedProps
}) => {
  let screenReaderTitle: string;
  if (pronunciation === "initialism") {
    screenReaderTitle = `${visualTitle} (${initializeText(children)})`;
  } else if (pronunciation === "acronym") {
    screenReaderTitle = `${visualTitle} (${flattenPronunciation(children)})`;
  } else {
    screenReaderTitle = visualTitle;
  }

  return (
    <abbr
      // Title attributes usually aren't natively available to screen readers;
      // always have to supplement with aria-label
      title={visualTitle}
      aria-label={screenReaderTitle}
      data-testid="abbr-root"
      css={{
        textDecoration: "inherit",
        letterSpacing: isAllUppercase(children) ? "0.02em" : "0",
      }}
      {...delegatedProps}
    >
      <span aria-hidden data-testid="abbr-visual-only">
        {children}
      </span>
    </abbr>
  );
};

function flattenPronunciation(text: string): string {
  const trimmed = text.trim();
  return (trimmed[0] ?? "").toUpperCase() + trimmed.slice(1).toLowerCase();
}

function initializeText(text: string): string {
  return text.trim().toUpperCase().replaceAll(/\B/g, ".") + ".";
}

function isAllUppercase(text: string): boolean {
  return text === text.toUpperCase();
}
