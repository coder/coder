/**
 * @file A more sophisticated version of the native <abbr> element.
 *
 * Features:
 * - Better type-safety (requiring you to include certain properties)
 * - All default styling is stripped away by default
 * - Better control over how screen readers read the text
 */
import { visuallyHidden } from "@mui/utils";
import { type FC } from "react";

type AbbrProps = {
  title: string;
  children: string;

  initialism?: boolean;
  className?: string;
};

export const Abbr: FC<AbbrProps> = ({
  title,
  className,
  children,
  initialism = false,
}) => {
  return (
    // Have to use test IDs instead of roles because traditional <abbr> elements
    // have weird edge cases and aren't that accessible, so abbreviated roles
    // usually aren't available in testing libraries
    <abbr
      title={title}
      css={[{ textDecoration: "inherit" }, className]}
      data-testid="abbr"
    >
      {initialism ? (
        // Helps make sure that screen readers read initialisms correctly
        // without it affecting the visual output for sighted users
        <>
          <span css={{ ...visuallyHidden }} data-testid="visually-hidden">
            {initializeText(children)}
          </span>

          <span aria-hidden data-testid="visual-only">
            {children}
          </span>
        </>
      ) : (
        children
      )}
    </abbr>
  );
};

function initializeText(text: string): string {
  return text.trim().toUpperCase().replaceAll(/\B/g, ".") + ".";
}
