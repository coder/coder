import SearchOutlined from "@mui/icons-material/SearchOutlined";
// eslint-disable-next-line no-restricted-imports -- use it to have the component prop
import Box, { type BoxProps } from "@mui/material/Box";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import visuallyHidden from "@mui/utils/visuallyHidden";
import {
  type FC,
  type HTMLAttributes,
  type InputHTMLAttributes,
  forwardRef,
} from "react";

export const Search = forwardRef<HTMLElement, BoxProps>(
  ({ children, ...boxProps }, ref) => {
    const theme = useTheme();

    return (
      <Box
        ref={ref}
        {...boxProps}
        css={{
          display: "flex",
          alignItems: "center",
          paddingLeft: 16,
          height: 40,
          borderBottom: `1px solid ${theme.palette.divider}`,
        }}
      >
        <SearchOutlined
          css={{
            fontSize: 14,
            color: theme.palette.text.secondary,
          }}
        />
        {children}
      </Box>
    );
  },
);

type SearchInputProps = InputHTMLAttributes<HTMLInputElement> & {
  label?: string;
};

export const SearchInput = forwardRef<HTMLInputElement, SearchInputProps>(
  ({ label, ...inputProps }, ref) => {
    const theme = useTheme();

    return (
      <>
        <label css={{ ...visuallyHidden }} htmlFor={inputProps.id}>
          {label}
        </label>
        <input
          ref={ref}
          tabIndex={-1}
          type="text"
          placeholder="Search..."
          css={{
            height: "100%",
            border: 0,
            background: "none",
            flex: 1,
            marginLeft: 16,
            outline: 0,
            "&::placeholder": {
              color: theme.palette.text.secondary,
            },
          }}
          {...inputProps}
        />
      </>
    );
  },
);

export const SearchEmpty: FC<HTMLAttributes<HTMLDivElement>> = ({
  children = "Not found",
  ...props
}) => {
  const theme = useTheme();

  return (
    <div
      css={{
        fontSize: 13,
        color: theme.palette.text.secondary,
        textAlign: "center",
        paddingTop: 8,
        paddingBottom: 8,
      }}
      {...props}
    >
      {children}
    </div>
  );
};

export const searchStyles = {
  content: {
    width: 320,
    padding: 0,
    borderRadius: 4,
  },
} satisfies Record<string, Interpolation<Theme>>;
