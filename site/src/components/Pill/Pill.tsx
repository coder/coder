import { type FC, type ReactNode, useMemo } from "react";
import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import { colors } from "theme/colors";

export type PillType =
  | "primary"
  | "secondary"
  | "error"
  | "warning"
  | "info"
  | "success"
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
  primary: (lightBorder) => ({
    backgroundColor: colors.blue[13],
    borderColor: lightBorder ? colors.blue[5] : colors.blue[7],
  }),
  secondary: (lightBorder) => ({
    backgroundColor: colors.indigo[13],
    borderColor: lightBorder ? colors.indigo[6] : colors.indigo[8],
  }),
  neutral: (lightBorder) => ({
    backgroundColor: colors.gray[13],
    borderColor: lightBorder ? colors.gray[6] : colors.gray[8],
  }),
} satisfies Record<string, (lightBorder?: boolean) => Interpolation<Theme>>;

const themeStyles =
  (type: PillType, lightBorder?: boolean) => (theme: Theme) => {
    const palette = theme.palette[type];
    return {
      backgroundColor: palette.dark,
      borderColor: lightBorder ? palette.light : palette.main,
    };
  };

export const Pill: FC<PillProps> = (props) => {
  const { lightBorder, icon, text = null, type = "neutral", ...attrs } = props;
  const theme = useTheme();

  const typeStyles = useMemo(() => {
    if (type in themeOverrides) {
      return themeOverrides[type as keyof typeof themeOverrides](lightBorder);
    }
    return themeStyles(type, lightBorder);
  }, [type, lightBorder]);

  return (
    <div
      css={[
        {
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
};
