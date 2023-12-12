import { useTheme } from "@emotion/react";
import Paper from "@mui/material/Paper";
import {
  type HTMLProps,
  type ReactNode,
  type FC,
  type PropsWithChildren,
} from "react";

export interface ChartSectionProps {
  /**
   * action appears in the top right of the section card
   */
  action?: ReactNode;
  contentsProps?: HTMLProps<HTMLDivElement>;
  title?: string | JSX.Element;
}

export const ChartSection: FC<PropsWithChildren<ChartSectionProps>> = ({
  action,
  children,
  contentsProps,
  title,
}) => {
  const theme = useTheme();

  return (
    <Paper
      css={{
        border: `1px solid ${theme.palette.divider}`,
        borderRadius: 8,
      }}
      elevation={0}
    >
      {title && (
        <div
          css={{
            alignItems: "center",
            display: "flex",
            justifyContent: "space-between",
            padding: "12px 16px",
          }}
        >
          <h6
            css={{
              margin: 0,
              fontSize: 14,
              fontWeight: 600,
            }}
          >
            {title}
          </h6>
          {action && <div>{action}</div>}
        </div>
      )}

      <div {...contentsProps} css={{ margin: 16 }}>
        {children}
      </div>
    </Paper>
  );
};
