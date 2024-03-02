import IconButton from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";
import { css, type Interpolation, type Theme } from "@emotion/react";
import { forwardRef, type ReactNode } from "react";
import { EyeIcon } from "../Icons/EyeIcon";
import { EyeClosedIcon } from "../Icons/EyeClosedIcon";

interface EyeButtonProps {
  children?: ReactNode;
  wrapperStyles?: Interpolation<Theme>;
  buttonStyles?: Interpolation<Theme>;
  tooltipTitle?: string;
  toShow:  boolean;
  toggleHide: () => void;
}

export const Language = {
  tooltipTitle: "Show/Hide Secret",
  ariaLabel: "Show/Hide Secret",
};

/**
 * Eye button used inside the CodeBlock component internally
 */
export const EyeButton = forwardRef<HTMLButtonElement, EyeButtonProps>(
  (props, ref) => {
    const {
      wrapperStyles,
      buttonStyles,
      tooltipTitle = Language.tooltipTitle,
      toggleHide,
      toShow
    } = props;

    return (
      <Tooltip title={tooltipTitle} placement="top">
        <div css={[{ display: "flex" }, wrapperStyles]}>
          <IconButton
            ref={ref}
            css={[styles.button, buttonStyles]}
            size="small"
            aria-label={Language.ariaLabel}
            variant="text"
            onClick={toggleHide}
          >
            {toShow ? (
              <EyeIcon css={styles.eyeIcon} />
            ) : (
              <EyeClosedIcon css={styles.eyeIcon} />
            )}
          </IconButton>
        </div>
      </Tooltip>
    );
  },
);

const styles = {
  button: (theme) => css`
    border-radius: 8px;
    padding: 8px;
    min-width: 32px;

    &:hover {
      background: ${theme.palette.background.paper};
    }
  `,
  eyeIcon: css`
    width: 20px;
    height: 20px;
  `,
} satisfies Record<string, Interpolation<Theme>>;
