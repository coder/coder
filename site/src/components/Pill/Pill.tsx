import { type FC, type ReactNode, useMemo, forwardRef } from "react";
import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import { colors } from "theme/colors";
import { dark } from "theme/theme";

// TODO: use a `ThemeRole` type or something
export type PillType =
  | "danger"
  | "error"
  | "warning"
  | "notice"
  | "info"
  | "success"
  | "active"
  | "neutral";

export interface PillProps {
  className?: string;
  icon?: ReactNode;
  text: ReactNode;
  type?: PillType;
  lightBorder?: boolean;
  title?: string;
}

const themeOverrides = {
  neutral: (lightBorder) => ({
    backgroundColor: colors.gray[13],
    borderColor: lightBorder ? colors.gray[6] : colors.gray[9],
  }),
} satisfies Record<string, (lightBorder?: boolean) => Interpolation<Theme>>;

const themeStyles =
  (type: Exclude<PillType, "neutral">, lightBorder?: boolean) =>
  (theme: Theme) => {
    const palette = dark.roles[type];
    return {
      backgroundColor: palette.background,
      borderColor: palette.outline,
      color: palette.text,
    };
  };

export const Pill: FC<PillProps> = forwardRef<HTMLDivElement, PillProps>(
  (props, ref) => {
    const {
      lightBorder,
      icon,
      text = null,
      type = "neutral",
      ...attrs
    } = props;
    const theme = useTheme();

    const typeStyles = useMemo(() => {
      if (type in themeOverrides) {
        return themeOverrides[type as keyof typeof themeOverrides](lightBorder);
      }
      // TODO: hack
      return themeStyles(type as any, lightBorder);
    }, [type, lightBorder]);

    return (
      <div
        ref={ref}
        css={[
          {
            cursor: "default",
            display: "inline-flex",
            alignItems: "center",
            borderWidth: 1,
            borderStyle: "solid",
            borderRadius: 99999,
            fontSize: 12,
            color: "#FFF",
            height: theme.spacing(3),
            paddingLeft: icon ? theme.spacing(0.75) : theme.spacing(1.5),
            paddingRight: theme.spacing(1.5),
            whiteSpace: "nowrap",
            fontWeight: 400,
          },
          typeStyles,
        ]}
        role="status"
        {...attrs}
      >
        {icon && (
          <div
            css={css`
              margin-right: ${theme.spacing(0.5)};
              width: ${theme.spacing(1.75)};
              height: ${theme.spacing(1.75)};
              line-height: 0;
              display: flex;
              align-items: center;
              justify-content: center;

              & > img,
              & > svg {
                width: ${theme.spacing(1.75)};
                height: ${theme.spacing(1.75)};
              }
            `}
          >
            {icon}
          </div>
        )}
        {text}
      </div>
    );
  },
);
