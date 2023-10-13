import Tooltip, { type TooltipProps } from "@mui/material/Tooltip";
import { useClickable } from "hooks/useClickable";
import { useClipboard } from "hooks/useClipboard";
import { type FC, type HTMLProps } from "react";

interface CopyableValueProps extends HTMLProps<HTMLDivElement> {
  value: string;
  placement?: TooltipProps["placement"];
  PopperProps?: TooltipProps["PopperProps"];
}

export const CopyableValue: FC<CopyableValueProps> = ({
  value,
  placement = "bottom-start",
  PopperProps,
  ...props
}) => {
  const { isCopied, copy } = useClipboard(value);
  const clickableProps = useClickable<HTMLSpanElement>(copy);

  return (
    <Tooltip
      title={isCopied ? "Copied!" : "Click to copy"}
      placement={placement}
      PopperProps={PopperProps}
    >
      <span {...props} {...clickableProps} css={{ cursor: "pointer" }} />
    </Tooltip>
  );
};
