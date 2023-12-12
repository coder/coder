import IconButton from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";
import Check from "@mui/icons-material/Check";
import { css, type Interpolation, type Theme } from "@emotion/react";
import { type FC, type ReactNode } from "react";
import { useClipboard } from "hooks/useClipboard";
import { FileCopyIcon } from "../Icons/FileCopyIcon";

interface CopyButtonProps {
  children?: ReactNode;
  text: string;
  ctaCopy?: string;
  wrapperStyles?: Interpolation<Theme>;
  buttonStyles?: Interpolation<Theme>;
  tooltipTitle?: string;
}

export const Language = {
  tooltipTitle: "Copy to clipboard",
  ariaLabel: "Copy to clipboard",
};

/**
 * Copy button used inside the CodeBlock component internally
 */
export const CopyButton: FC<CopyButtonProps> = ({
  text,
  ctaCopy,
  wrapperStyles,
  buttonStyles,
  tooltipTitle = Language.tooltipTitle,
}) => {
  const { isCopied, copy: copyToClipboard } = useClipboard(text);

  return (
    <Tooltip title={tooltipTitle} placement="top">
      <div css={[{ display: "flex" }, wrapperStyles]}>
        <IconButton
          css={[
            (theme) => css`
              border-radius: 8px;
              padding: 8px;
              min-width: 32px;

              &:hover {
                background: ${theme.palette.background.paper};
              }
            `,
            buttonStyles,
          ]}
          onClick={copyToClipboard}
          size="small"
          aria-label={Language.ariaLabel}
          variant="text"
        >
          {isCopied ? (
            <Check css={styles.copyIcon} />
          ) : (
            <FileCopyIcon css={styles.copyIcon} />
          )}
          {ctaCopy && <div css={{ marginLeft: 8 }}>{ctaCopy}</div>}
        </IconButton>
      </div>
    </Tooltip>
  );
};

const styles = {
  copyIcon: css`
    width: 20px;
    height: 20px;
  `,
} satisfies Record<string, Interpolation<Theme>>;
