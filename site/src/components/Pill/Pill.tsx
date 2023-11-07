import { type FC, type ReactNode, useMemo, forwardRef } from "react";
import { css, type Interpolation, type Theme } from "@emotion/react";
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
            height: 24,
            paddingLeft: icon ? 6 : 12,
            paddingRight: 12,
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
              margin-right: 4px;
              width: 14px;
              height: 14px;
              line-height: 0;
              display: flex;
              align-items: center;
              justify-content: center;

              & > img,
              & > svg {
                width: 14px;
                height: 14px;
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
