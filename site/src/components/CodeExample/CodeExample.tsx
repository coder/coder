import { type FC } from "react";
import { type Interpolation, type Theme } from "@emotion/react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { CopyButton } from "../CopyButton/CopyButton";

export interface CodeExampleProps {
  code: string;
  secret?: boolean;
  className?: string;
}

/**
 * Component to show single-line code examples, with a copy button
 */
export const CodeExample: FC<CodeExampleProps> = ({
  code,
  secret,
  className,
}) => {
  return (
    <div css={styles.container} className={className}>
      <code css={[styles.code, secret && styles.secret]}>{code}</code>
      <CopyButton text={code} />
    </div>
  );
};

const styles = {
  container: (theme) => ({
    display: "flex",
    flexDirection: "row",
    alignItems: "center",
    color: theme.experimental.l1.text,
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 14,
    borderRadius: 8,
    padding: 8,
    lineHeight: "150%",
    border: `1px solid ${theme.experimental.l1.outline}`,
  }),

  code: {
    padding: "0 8px",
    flexGrow: 1,
    wordBreak: "break-all",
  },

  secret: {
    "-webkit-text-security": "disc", // also supported by firefox
  },
} satisfies Record<string, Interpolation<Theme>>;
