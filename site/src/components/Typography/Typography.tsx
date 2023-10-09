/**
 * @fileoverview (TODO: Grey) This file is in a temporary state and is a
 * verbatim port from `@coder/ui`.
 */

import MuiTypography, {
  TypographyProps as MuiTypographyProps,
} from "@mui/material/Typography";
import * as React from "react";

export interface TypographyProps extends MuiTypographyProps {
  short?: boolean;
}

/**
 * Wrapper around Material UI's Typography component to allow for future
 * custom typography types.
 *
 * See original component's Material UI documentation here: https://material-ui.com/components/typography/
 */
export const Typography: React.FC<TypographyProps> = ({ short, ...attrs }) => {
  return (
    <MuiTypography
      css={[
        short && {
          "&.MuiTypography-body1": {
            lineHeight: "21px",
          },
          "&.MuiTypography-body2": {
            lineHeight: "18px",
            letterSpacing: 0.2,
          },
        },
      ]}
      {...attrs}
    />
  );
};
