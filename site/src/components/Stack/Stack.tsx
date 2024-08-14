import type { CSSObject } from "@emotion/react";
import { forwardRef } from "react";

export type StackProps = {
  className?: string;
  direction?: "column" | "row";
  spacing?: number;
  alignItems?: CSSObject["alignItems"];
  justifyContent?: CSSObject["justifyContent"];
  wrap?: CSSObject["flexWrap"];
} & React.HTMLProps<HTMLDivElement>;

export const Stack = forwardRef<HTMLDivElement, StackProps>((props, ref) => {
  const {
    children,
    direction = "column",
    spacing = 2,
    alignItems,
    justifyContent,
    wrap,
    ...divProps
  } = props;

  return (
    <div
      {...divProps}
      ref={ref}
      css={{
        display: "flex",
        flexDirection: direction,
        gap: spacing * 8,
        alignItems: alignItems,
        justifyContent: justifyContent,
        flexWrap: wrap,
        maxWidth: "100%",
      }}
    >
      {children}
    </div>
  );
});
