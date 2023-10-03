import IconButton from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";
import Check from "@mui/icons-material/Check";
import { useClipboard } from "hooks/useClipboard";
import { css } from "@emotion/react";
import { FileCopyIcon } from "../Icons/FileCopyIcon";

interface CopyButtonProps {
  text: string;
  ctaCopy?: string;
  wrapperClassName?: string;
  buttonClassName?: string;
  tooltipTitle?: string;
}

export const Language = {
  tooltipTitle: "Copy to clipboard",
  ariaLabel: "Copy to clipboard",
};

/**
 * Copy button used inside the CodeBlock component internally
 */
export const CopyButton: React.FC<React.PropsWithChildren<CopyButtonProps>> = ({
  text,
  ctaCopy,
  wrapperClassName = "",
  buttonClassName = "",
  tooltipTitle = Language.tooltipTitle,
}) => {
  const { isCopied, copy: copyToClipboard } = useClipboard(text);

  const fileCopyIconStyles = css`
    width: 20px;
    height: 20px;
  `;

  return (
    <Tooltip title={tooltipTitle} placement="top">
      <div
        className={wrapperClassName}
        css={{
          display: "flex",
        }}
      >
        <IconButton
          className={buttonClassName}
          css={(theme) => css`
            border-radius: ${theme.shape.borderRadius}px;
            padding: ${theme.spacing(0.85)};
            min-width: 32px;

            &:hover {
              background: ${theme.palette.background.paper};
            }
          `}
          onClick={copyToClipboard}
          size="small"
          aria-label={Language.ariaLabel}
          variant="text"
        >
          {isCopied ? (
            <Check css={fileCopyIconStyles} />
          ) : (
            <FileCopyIcon css={fileCopyIconStyles} />
          )}
          {ctaCopy && (
            <div
              css={(theme) => ({
                marginLeft: theme.spacing(1),
              })}
            >
              {ctaCopy}
            </div>
          )}
        </IconButton>
      </div>
    </Tooltip>
  );
};
