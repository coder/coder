/* eslint-disable jsx-a11y/heading-has-content -- infer from props */
import {
  ComponentProps,
  HTMLAttributes,
  ReactElement,
  cloneElement,
  forwardRef,
} from "react";
import CheckCircleOutlined from "@mui/icons-material/CheckCircleOutlined";
import ErrorOutline from "@mui/icons-material/ErrorOutline";
import { healthyColor } from "./healthyColor";
import { css } from "@emotion/css";
import DoNotDisturbOnOutlined from "@mui/icons-material/DoNotDisturbOnOutlined";
import { HealthSeverity } from "api/typesGenerated";
import { useTheme } from "@mui/material/styles";

const CONTENT_PADDING = 36;

export const Header = (props: HTMLAttributes<HTMLDivElement>) => {
  return (
    <header
      css={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        padding: `36px ${CONTENT_PADDING}px`,
      }}
      {...props}
    />
  );
};

export const HeaderTitle = (props: HTMLAttributes<HTMLHeadingElement>) => {
  return (
    <h2
      css={{
        margin: 0,
        lineHeight: "1.2",
        fontSize: 20,
        fontWeight: 500,
        display: "flex",
        alignItems: "center",
        gap: 16,
      }}
      {...props}
    />
  );
};

export const HealthIcon = ({
  size,
  severity,
}: {
  size: number;
  severity: HealthSeverity;
}) => {
  const theme = useTheme();
  const color = healthyColor(theme, severity);
  const Icon = severity === "error" ? ErrorOutline : CheckCircleOutlined;

  return <Icon css={{ width: size, height: size, color }} />;
};

export const HealthyDot = ({ severity }: { severity: HealthSeverity }) => {
  const theme = useTheme();

  return (
    <div
      css={{
        width: 8,
        height: 8,
        borderRadius: 9999,
        backgroundColor: healthyColor(theme, severity),
      }}
    />
  );
};

export const Main = (props: HTMLAttributes<HTMLDivElement>) => {
  return (
    <main
      css={{
        padding: `0 ${CONTENT_PADDING}px ${CONTENT_PADDING}px`,
        display: "flex",
        flexDirection: "column",
        gap: 36,
      }}
      {...props}
    />
  );
};

export const GridData = (props: HTMLAttributes<HTMLDivElement>) => {
  return (
    <div
      css={{
        lineHeight: "1.4",
        display: "grid",
        gridTemplateColumns: "auto auto",
        gap: 12,
        columnGap: 48,
        width: "min-content",
        whiteSpace: "nowrap",
      }}
      {...props}
    />
  );
};

export const GridDataLabel = (props: HTMLAttributes<HTMLSpanElement>) => {
  const theme = useTheme();
  return (
    <span
      css={{
        fontSize: 14,
        fontWeight: 500,
        color: theme.palette.text.secondary,
      }}
      {...props}
    />
  );
};

export const GridDataValue = (props: HTMLAttributes<HTMLSpanElement>) => {
  const theme = useTheme();
  return (
    <span
      css={{
        fontSize: 14,
        color: theme.palette.text.primary,
      }}
      {...props}
    />
  );
};

export const SectionLabel = (props: HTMLAttributes<HTMLHeadingElement>) => {
  return (
    <h4
      {...props}
      css={{
        fontSize: 14,
        fontWeight: 500,
        margin: 0,
        lineHeight: "1.2",
        marginBottom: 16,
      }}
    />
  );
};

type PillProps = HTMLAttributes<HTMLDivElement> & {
  icon: ReactElement;
};

export const Pill = forwardRef<HTMLDivElement, PillProps>((props, ref) => {
  const theme = useTheme();
  const { icon, children, ...divProps } = props;

  return (
    <div
      ref={ref}
      css={{
        display: "inline-flex",
        alignItems: "center",
        height: 32,
        borderRadius: 9999,
        border: `1px solid ${theme.palette.divider}`,
        fontSize: 12,
        fontWeight: 500,
        padding: "8px 16px 8px 8px",
        gap: 8,
        cursor: "default",
      }}
      {...divProps}
    >
      {cloneElement(icon, { className: css({ width: 14, height: 14 }) })}
      {children}
    </div>
  );
});

type BooleanPillProps = Omit<
  ComponentProps<typeof Pill>,
  "children" | "icon" | "value"
> & {
  children: string;
  value: boolean;
};

export const BooleanPill = (props: BooleanPillProps) => {
  const { value, children, ...divProps } = props;
  const theme = useTheme();
  const color = value ? theme.palette.success.light : theme.palette.error.light;

  return (
    <Pill
      icon={
        value ? (
          <CheckCircleOutlined css={{ color }} />
        ) : (
          <DoNotDisturbOnOutlined css={{ color }} />
        )
      }
      {...divProps}
    >
      {children}
    </Pill>
  );
};

type LogsProps = { lines: string[] } & HTMLAttributes<HTMLDivElement>;

export const Logs = (props: LogsProps) => {
  const theme = useTheme();
  const { lines, ...divProps } = props;

  return (
    <div
      css={{
        fontFamily: "monospace",
        fontSize: 13,
        lineHeight: "160%",
        padding: 24,
        backgroundColor: theme.palette.background.paper,
        overflowX: "auto",
        whiteSpace: "pre-wrap",
        wordBreak: "break-all",
      }}
      {...divProps}
    >
      {lines.map((line, index) => (
        <span css={{ display: "block" }} key={index}>
          {line}
        </span>
      ))}
      {lines.length === 0 && (
        <span css={{ color: theme.palette.text.secondary }}>
          No logs available
        </span>
      )}
    </div>
  );
};
