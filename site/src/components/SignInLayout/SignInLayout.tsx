import { type FC, type ReactNode } from "react";

export const SignInLayout: FC<{ children: ReactNode }> = ({ children }) => {
  return (
    <div
      css={{
        flex: 1,
        height: "-webkit-fill-available",
        display: "flex",
        justifyContent: "center",
        alignItems: "center",
      }}
    >
      <div
        css={{
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
        }}
      >
        <div
          css={{
            maxWidth: 385,
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
          }}
        >
          {children}
        </div>
        <div
          css={(theme) => ({
            fontSize: 12,
            color: theme.palette.text.secondary,
            marginTop: theme.spacing(3),
          })}
        >
          {`\u00a9 ${new Date().getFullYear()} Coder Technologies, Inc.`}
        </div>
      </div>
    </div>
  );
};
