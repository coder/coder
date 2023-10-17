import Typography from "@mui/material/Typography";
import { type FC, type PropsWithChildren } from "react";
import { css, useTheme } from "@emotion/react";
import { CoderIcon } from "../Icons/CoderIcon";

const Language = {
  defaultMessage: (
    <>
      Welcome to <strong>Coder</strong>
    </>
  ),
};

export const Welcome: FC<
  PropsWithChildren<{ message?: JSX.Element | string }>
> = ({ message = Language.defaultMessage }) => {
  const theme = useTheme();

  return (
    <div>
      <div
        css={{
          display: "flex",
          justifyContent: "center",
        }}
      >
        <CoderIcon
          css={{
            color: theme.palette.text.primary,
            fontSize: theme.spacing(8),
          }}
        />
      </div>
      <Typography
        css={css`
          text-align: center;
          font-size: ${theme.spacing(4)};
          font-weight: 400;
          margin: 0;
          margin-bottom: ${theme.spacing(2)};
          margin-top: ${theme.spacing(2)};
          line-height: 1.25;

          & strong {
            font-weight: 600;
          }
        `}
        variant="h1"
      >
        {message}
      </Typography>
    </div>
  );
};
