/**
 * @file A more sophisticated version of the native <abbr> element.
 *
 * Features:
 * - Better type-safety (requiring you to include certain properties)
 * - All built-in HTML styling is stripped away by default
 * - Better integration with screen readers, with more options for controlling
 *   how they read out initialisms.
 */
import { visuallyHidden } from "@mui/utils";
import { type FC, type HTMLAttributes } from "react";

// There isn't a dedicated HTMLAbbrElement type; have to go with base type
type AbbrProps = Exclude<HTMLAttributes<HTMLElement>, "title" | "children"> & {
  // Not calling this "title" to make it clear that it doesn't have the same
  // issues as the native title attribute as far as screen reader support
  expandedText: string;
  children: string;
  initialism?: boolean;
};

export const Abbr: FC<AbbrProps> = ({
  children,
  expandedText,
  initialism = false,
  ...delegatedProps
}) => {
  return (
    // Have to use test IDs instead of roles because traditional <abbr> elements
    // have weird edge cases and aren't that accessible, so abbreviated roles
    // usually aren't available in testing libraries
    <abbr
      // Title attributes usually aren't natively available to screen readers;
      // still have to inject text manually. Main value of titles here is
      // letting sighted users hover over the abbreviation to see expanded text
      title={expandedText}
      data-testid="abbr"
      css={{
        textDecoration: "inherit",
        // Rare case where this should be ems, not rems
        letterSpacing: isAllUppercase(children) ? "0.02em" : "0",
      }}
      {...delegatedProps}
    >
      {initialism ? (
        // Helps make sure that screen readers read initialisms correctly
        // without it affecting the visual output for sighted users (e.g.,
        // making sure "CLI" isn't read out as "klee")
        <>
          {/*
           * Once speakAs: "spell-out" has more browser support, that CSS
           * property can be swapped in and clean up this code a lot
           */}
          <span css={{ ...visuallyHidden }} data-testid="visually-hidden">
            {initializeText(children)}
          </span>
          <span aria-hidden data-testid="visual-only">
            {children}
          </span>
        </>
      ) : (
        <span aria-label={expandedText}>{children}</span>
      )}
    </abbr>
  );
};

function initializeText(text: string): string {
  return text.trim().toUpperCase().replaceAll(/\B/g, ".") + ".";
}

function isAllUppercase(text: string): boolean {
  return text === text.toUpperCase();
}
