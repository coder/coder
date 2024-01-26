import { useRef, type FC } from "react";
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
  const buttonRef = useRef<HTMLButtonElement>(null);
  const triggerButton = () => buttonRef.current?.click();

  return (
    /* eslint-disable-next-line jsx-a11y/no-static-element-interactions --
       Expanding clickable area of CodeExample for better ergonomics, but don't
       want to change the semantics of the HTML elements being rendered
    */
    <div
      css={styles.container}
      className={className}
      onClick={(event) => {
        if (event.target !== buttonRef.current) {
          triggerButton();
        }
      }}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          triggerButton();
        }
      }}
      onKeyUp={(event) => {
        if (event.key === " ") {
          triggerButton();
        }
      }}
    >
      <code css={[styles.code, secret && styles.secret]}>{code}</code>
      <CopyButton ref={buttonRef} text={code} />
    </div>
  );
};

const styles = {
  container: (theme) => ({
    cursor: "pointer",
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

    "&:hover": {
      backgroundColor: theme.experimental.l2.hover.background,
    },
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
