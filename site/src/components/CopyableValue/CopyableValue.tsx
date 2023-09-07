import { makeStyles } from "@mui/styles";
import Tooltip from "@mui/material/Tooltip";
import { useClickable } from "hooks/useClickable";
import { useClipboard } from "hooks/useClipboard";
import { FC, HTMLProps } from "react";
import { combineClasses } from "utils/combineClasses";

interface CopyableValueProps extends HTMLProps<HTMLDivElement> {
  value: string;
}

export const CopyableValue: FC<CopyableValueProps> = ({
  value,
  className,
  ...props
}) => {
  const { isCopied, copy } = useClipboard(value);
  const clickableProps = useClickable(copy);
  const styles = useStyles();

  return (
    <Tooltip
      title={isCopied ? "Copied!" : "Click to copy"}
      placement="bottom-start"
    >
      <span
        {...props}
        {...clickableProps}
        className={combineClasses([styles.value, className])}
      />
    </Tooltip>
  );
};

const useStyles = makeStyles(() => ({
  value: {
    cursor: "pointer",
  },
}));
