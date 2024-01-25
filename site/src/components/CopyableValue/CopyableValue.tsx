import Tooltip, { type TooltipProps } from "@mui/material/Tooltip";
import { type FC, type HTMLAttributes } from "react";
import { useClickable } from "hooks/useClickable";
import { useClipboard } from "hooks/useClipboard";

interface CopyableValueProps extends HTMLAttributes<HTMLSpanElement> {
  value: string;
  placement?: TooltipProps["placement"];
  PopperProps?: TooltipProps["PopperProps"];
}

export const CopyableValue: FC<CopyableValueProps> = ({
  value,
  placement = "bottom-start",
  PopperProps,
  children,
  ...attrs
}) => {
  const { isCopied, copy } = useClipboard(value);
  const clickableProps = useClickable<HTMLSpanElement>(copy);

  return (
    <Tooltip
      title={isCopied ? "Copied!" : "Click to copy"}
      placement={placement}
      PopperProps={PopperProps}
    >
      <span {...attrs} {...clickableProps} css={{ cursor: "pointer" }}>
        {children}
      </span>
    </Tooltip>
  );
};
