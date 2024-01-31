import { type FC, type ReactNode, forwardRef, HTMLAttributes } from "react";
import { useTheme, type Theme } from "@emotion/react";
import type { ThemeRole } from "theme/experimental";
import CircularProgress, {
  CircularProgressProps,
} from "@mui/material/CircularProgress";

export type PillProps = HTMLAttributes<HTMLDivElement> & {
  icon?: ReactNode;
  color?: ThemeRole;
};

const themeStyles = (color: ThemeRole) => (theme: Theme) => {
  const palette = theme.experimental.roles[color];

  return {
    backgroundColor: palette.background,
    borderColor: palette.outline,
  };
};

const PILL_HEIGHT = 24;
const PILL_ICON_SIZE = 14;
const PILL_ICON_SPACING = (PILL_HEIGHT - PILL_ICON_SIZE) / 2;

export const Pill: FC<PillProps> = forwardRef<HTMLDivElement, PillProps>(
  (props, ref) => {
    const { icon, color = "neutral", children, ...divProps } = props;
    const theme = useTheme();
    const typeStyles = themeStyles(color);

    return (
      <div
        ref={ref}
        css={[
          {
            fontSize: 12,
            color: theme.experimental.l1.text,
            cursor: "default",
            display: "inline-flex",
            alignItems: "center",
            whiteSpace: "nowrap",
            fontWeight: 400,
            borderWidth: 1,
            borderStyle: "solid",
            borderRadius: 99999,
            lineHeight: 1,
            height: PILL_HEIGHT,
            gap: PILL_ICON_SPACING,
            paddingRight: 12,
            paddingLeft: icon ? PILL_ICON_SPACING : 12,

            "& svg": {
              width: PILL_ICON_SIZE,
              height: PILL_ICON_SIZE,
            },
          },
          typeStyles,
        ]}
        {...divProps}
      >
        {icon}
        {children}
      </div>
    );
  },
);

export const PillSpinner = (props: CircularProgressProps) => {
  return (
    <CircularProgress
      size={PILL_ICON_SIZE}
      css={(theme) => ({
        color: theme.experimental.l1.text,
        // It is necessary to align it with the MUI Icons internal padding
        "& svg": {
          transform: "scale(.75)",
        },
      })}
      {...props}
    />
  );
};
