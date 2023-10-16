import { type FC } from "react";
import { useTheme } from "@emotion/react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { CopyButton } from "../CopyButton/CopyButton";

export interface CodeExampleProps {
  code: string;
  password?: boolean;
  className?: string;
}

/**
 * Component to show single-line code examples, with a copy button
 */
export const CodeExample: FC<CodeExampleProps> = (props) => {
  const { code, password, className } = props;
  const theme = useTheme();

  return (
    <div
      css={{
        display: "flex",
        flexDirection: "row",
        alignItems: "center",
        background: "rgb(0 0 0 / 30%)",
        color: theme.palette.primary.contrastText,
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontSize: 14,
        borderRadius: theme.shape.borderRadius,
        padding: theme.spacing(1),
        lineHeight: "150%",
        border: `1px solid ${theme.palette.divider}`,
      }}
      className={className}
    >
      <code
        css={{
          padding: theme.spacing(0, 1),
          width: "100%",
          display: "flex",
          alignItems: "center",
          wordBreak: "break-all",
          "-webkit-text-security": password ? "disc" : undefined,
        }}
      >
        {code}
      </code>
      <CopyButton text={code} />
    </div>
  );
};
