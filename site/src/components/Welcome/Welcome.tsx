import { type FC, type PropsWithChildren } from "react";
import { useTheme } from "@emotion/react";
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
            fontSize: 64,
          }}
        />
      </div>
      <h1
        css={{
          textAlign: "center",
          fontSize: 32,
          fontWeight: 400,
          margin: 0,
          marginTop: 16,
          marginBottom: 32,
          lineHeight: 1.25,

          "& strong": {
            fontWeight: 600,
          },
        }}
      >
        {message}
      </h1>
    </div>
  );
};
