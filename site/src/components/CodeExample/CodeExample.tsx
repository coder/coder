import { makeStyles } from "@mui/styles";
import { FC } from "react";
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants";
import { combineClasses } from "../../utils/combineClasses";
import { CopyButton } from "../CopyButton/CopyButton";
import { Theme } from "@mui/material/styles";

export interface CodeExampleProps {
  code: string;
  className?: string;
  buttonClassName?: string;
  tooltipTitle?: string;
  inline?: boolean;
  password?: boolean;
}

/**
 * Component to show single-line code examples, with a copy button
 */
export const CodeExample: FC<React.PropsWithChildren<CodeExampleProps>> = ({
  code,
  className,
  buttonClassName,
  tooltipTitle,
  inline,
}) => {
  const styles = useStyles({ inline: inline });

  return (
    <div className={combineClasses([styles.root, className])}>
      <code className={styles.code}>{code}</code>
      <CopyButton
        text={code}
        tooltipTitle={tooltipTitle}
        buttonClassName={buttonClassName}
      />
    </div>
  );
};

interface styleProps {
  inline?: boolean;
  password?: boolean;
}

const useStyles = makeStyles<Theme, styleProps>((theme) => ({
  root: (props) => ({
    display: props.inline ? "inline-flex" : "flex",
    flexDirection: "row",
    alignItems: "center",
    background: props.inline ? "rgb(0 0 0 / 30%)" : "hsl(223, 27%, 3%)",
    border: props.inline ? undefined : `1px solid ${theme.palette.divider}`,
    color: theme.palette.primary.contrastText,
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 14,
    borderRadius: theme.shape.borderRadius,
    padding: theme.spacing(1),
    lineHeight: "150%",
  }),
  code: {
    padding: theme.spacing(0, 1),
    width: "100%",
    display: "flex",
    alignItems: "center",
    wordBreak: "break-all",
    "-webkit-text-security": (props) => (props.password ? "disc" : undefined),
  },
}));
