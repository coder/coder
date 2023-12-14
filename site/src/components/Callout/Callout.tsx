import { type FC, type ReactNode } from "react";
import { useTheme } from "@emotion/react";

// TODO: use a `ThemeRole` type or something
export type CalloutType =
  | "danger"
  | "error"
  | "warning"
  | "notice"
  | "info"
  | "success"
  | "active";
// | "neutral";

export interface CalloutProps {
  children?: ReactNode;
  icon?: ReactNode | boolean;
  type: CalloutType;
}

export const Callout: FC<CalloutProps> = ({ children, type }) => {
  const theme = useTheme();

  return (
    <div
      css={{
        backgroundColor: theme.experimental.roles[type].background,
        border: `1px solid ${theme.experimental.roles[type].outline}`,
        borderRadius: theme.shape.borderRadius,
        color: theme.experimental.roles[type].text,
        padding: "8px 16px",
      }}
    >
      {children}
    </div>
  );
};
