import { type FC, type HTMLAttributes } from "react";

export type Pronunciation = "shorthand" | "acronym" | "initialism";

type AbbrProps = HTMLAttributes<HTMLElement> & {
  children: string;
  title: string;
  pronunciation?: Pronunciation;
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
  title,
  pronunciation = "shorthand",
  ...delegatedProps
}) => {
  return (
    <abbr
      // Title attributes usually aren't natively available to screen readers;
      // always have to supplement with aria-label
      title={title}
      aria-label={getAccessibleLabel(children, title, pronunciation)}
      css={{
        textDecoration: "inherit",
        letterSpacing: children === children.toUpperCase() ? "0.02em" : "0",
      }}
      {...delegatedProps}
    >
      <span aria-hidden>{children}</span>
    </abbr>
  );
};

function getAccessibleLabel(
  abbreviation: string,
  title: string,
  pronunciation: Pronunciation,
): string {
  if (pronunciation === "initialism") {
    return `${initializeText(abbreviation)} (${title})`;
  }

  if (pronunciation === "acronym") {
    return `${flattenPronunciation(abbreviation)} (${title})`;
  }

  return title;
}

function initializeText(text: string): string {
  return text.trim().toUpperCase().replaceAll(/\B/g, ".") + ".";
}

function flattenPronunciation(text: string): string {
  const trimmed = text.trim();
  return (trimmed[0] ?? "").toUpperCase() + trimmed.slice(1).toLowerCase();
}
