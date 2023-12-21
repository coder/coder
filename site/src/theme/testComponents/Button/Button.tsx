import { css, useTheme } from "@emotion/react";
import { type FC } from "react";
import {
  type InteractiveThemeRole,
  type InteractiveRole,
} from "theme/experimental";

interface TestButtonProps {
  type: InteractiveThemeRole;
  variant?: "static" | "hover" | "disabled";
}

export const TestButton: FC<TestButtonProps> = ({ type, variant }) => {
  const theme = useTheme();
  const themeRole = theme.experimental.roles[type];

  return (
    <button
      css={[
        styles.base,
        variant ? styles[variant](themeRole) : styles.default(themeRole),
      ]}
      disabled={variant === "disabled"}
    >
      Do the thing
    </button>
  );
};

const styles = {
  base: css({
    fontWeight: 500,
    transition:
      "background 200ms ease, border 200ms ease, color 200ms ease, filter 200ms ease",
    borderRadius: 8,
    fontSize: 14,
    padding: "6px 12px",
  }),

  static: (themeRole: InteractiveRole) => ({
    background: themeRole.background,
    color: themeRole.text,
    border: `1px solid ${themeRole.outline}`,
  }),

  hover: (themeRole: InteractiveRole) => ({
    filter: "brightness(95%)",
    background: themeRole.hover.background,
    color: themeRole.hover.text,
    border: `1px solid ${themeRole.hover.outline}`,
  }),

  disabled: (themeRole: InteractiveRole) => ({
    background: themeRole.disabled.background,
    color: themeRole.disabled.text,
    border: `1px solid ${themeRole.disabled.outline}`,
  }),

  default: (themeRole: InteractiveRole) => ({
    ...styles.static(themeRole),
    "&:hover": styles.hover(themeRole),
    "&:disabled": styles.disabled(themeRole),
  }),
};
