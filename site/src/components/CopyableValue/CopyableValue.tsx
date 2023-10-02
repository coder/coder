import Tooltip from "@mui/material/Tooltip";
import { useClickable } from "hooks/useClickable";
import { useClipboard } from "hooks/useClipboard";
import { FC, HTMLProps } from "react";

interface CopyableValueProps extends HTMLProps<HTMLDivElement> {
  value: string;
}

export const CopyableValue: FC<CopyableValueProps> = ({ value, ...props }) => {
  const { isCopied, copy } = useClipboard(value);
  const clickableProps = useClickable<HTMLSpanElement>(copy);

  return (
    <Tooltip
      title={isCopied ? "Copied!" : "Click to copy"}
      placement="bottom-start"
    >
      <span {...props} {...clickableProps} css={{ cursor: "pointer" }} />
    </Tooltip>
  );
};
