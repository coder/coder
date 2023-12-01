/* eslint-disable jsx-a11y/heading-has-content -- infer from props */
import { HTMLProps } from "react";

export const Header = (props: HTMLProps<HTMLDivElement>) => {
  return (
    <header
      css={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        padding: "24px 36px",
      }}
      {...props}
    />
  );
};

export const HeaderTitle = (props: HTMLProps<HTMLHeadingElement>) => {
  return (
    <h2
      css={{ margin: 0, lineHeight: "120%", fontSize: 20, fontWeight: 500 }}
      {...props}
    />
  );
};
