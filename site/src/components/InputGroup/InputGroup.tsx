import type { FC, HTMLProps } from "react";

export const InputGroup: FC<HTMLProps<HTMLDivElement>> = (props) => {
  return (
    <div
      {...props}
      css={{
        "&": {
          display: "flex",
          alignItems: "flex-start",
        },

        "& > *:hover": {
          zIndex: 1,
        },

        "& > *:not(:last-child)": {
          marginRight: -1,
        },

        "& > *:first-child": {
          borderTopRightRadius: 0,
          borderBottomRightRadius: 0,

          "&.MuiFormControl-root .MuiInputBase-root": {
            borderTopRightRadius: 0,
            borderBottomRightRadius: 0,
          },
        },

        "& > *:last-child": {
          borderTopLeftRadius: 0,
          borderBottomLeftRadius: 0,

          "&.MuiFormControl-root .MuiInputBase-root": {
            borderTopLeftRadius: 0,
            borderBottomLeftRadius: 0,
          },
        },

        "& > *:not(:first-child):not(:last-child)": {
          borderRadius: 0,

          "&.MuiFormControl-root .MuiInputBase-root": {
            borderRadius: 0,
          },
        },

        "& .Mui-focused, & .Mui-error": {
          zIndex: 2,
        },
      }}
    />
  );
};
