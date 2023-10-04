import type { FC } from "react";
import type { CSSObject } from "@emotion/react";

export type StackProps = {
  className?: string;
  direction?: "column" | "row";
  spacing?: number;
  alignItems?: CSSObject["alignItems"];
  justifyContent?: CSSObject["justifyContent"];
  maxWidth?: CSSObject["maxWidth"];
  wrap?: CSSObject["flexWrap"];
} & React.HTMLProps<HTMLDivElement>;

export const Stack: FC<StackProps> = (props) => {
  const {
    children,
    direction = "column",
    spacing = 2,
    alignItems,
    justifyContent,
    maxWidth,
    wrap,
    ...divProps
  } = props;

  return (
    <div
      {...divProps}
      css={(theme) => ({
        display: "flex",
        flexDirection: direction,
        gap: spacing && theme.spacing(spacing),
        alignItems: alignItems,
        justifyContent: justifyContent,
        flexWrap: wrap,
        maxWidth: maxWidth,
      })}
    >
      {children}
    </div>
  );
};
