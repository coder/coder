import { type FC, type ReactNode, useMemo, forwardRef } from "react";
import { css, type Interpolation, type Theme } from "@emotion/react";
import { colors } from "theme/colors";
import type { ThemeRole } from "theme/experimental";

export type PillType = ThemeRole | keyof typeof themeOverrides;

export interface PillProps {
  className?: string;
  icon?: ReactNode;
  text: ReactNode;
  type?: PillType;
  title?: string;
}

const themeOverrides = {
  neutral: {
    backgroundColor: colors.gray[13],
    borderColor: colors.gray[6],
  },
} satisfies Record<string, Interpolation<Theme>>;

const themeStyles = (type: ThemeRole) => (theme: Theme) => {
  const palette = theme.experimental.roles[type];
  return {
    backgroundColor: palette.background,
    borderColor: palette.outline,
  };
};

export const Pill: FC<PillProps> = forwardRef<HTMLDivElement, PillProps>(
  (props, ref) => {
    const { icon, text = null, type = "neutral", ...attrs } = props;

    const typeStyles = useMemo(() => {
      if (type in themeOverrides) {
        return themeOverrides[type as keyof typeof themeOverrides];
      }
      return themeStyles(type as ThemeRole);
    }, [type]);

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
