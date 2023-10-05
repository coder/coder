/**
 * @file Defines a controlled searchbox component for processing form state.
 *
 * Not defined as a top-level component just yet, because it's not clear how
 * reusable this is outside of workspace dropdowns.
 */
import {
  type ForwardedRef,
  type KeyboardEvent,
  forwardRef,
  useId,
} from "react";

import Box from "@mui/system/Box";
import SearchIcon from "@mui/icons-material/SearchOutlined";
import { visuallyHidden } from "@mui/utils";
import { type SystemStyleObject } from "@mui/system";

type Props = {
  value: string;
  onValueChange: (newValue: string) => void;

  placeholder?: string;
  label?: string;
  onKeyDown?: (event: KeyboardEvent) => void;
  sx?: SystemStyleObject;
};

export const SearchBox = forwardRef(function SearchBox(
  {
    value,
    onValueChange,
    onKeyDown,
    label = "Search",
    placeholder = "Search...",
    sx = {},
  }: Props,
  ref?: ForwardedRef<HTMLInputElement>,
) {
  const hookId = useId();
  const inputId = `${hookId}-${SearchBox.name}-input`;

  return (
    <Box
      sx={{
        display: "flex",
        flexFlow: "row nowrap",
        alignItems: "center",
        paddingX: 2,
        height: "40px",
        borderBottom: (theme) => `1px solid ${theme.palette.divider}`,
        ...sx,
      }}
      onKeyDown={onKeyDown}
    >
      <Box component="div" sx={{ width: "18px" }}>
        <SearchIcon
          sx={{
            display: "block",
            fontSize: "14px",
            marginX: "auto",
            color: (theme) => theme.palette.text.secondary,
          }}
        />
      </Box>

      <Box component="label" sx={visuallyHidden} htmlFor={inputId}>
        {label}
      </Box>

      <Box
        component="input"
        type="text"
        ref={ref}
        id={inputId}
        autoFocus
        tabIndex={0}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onValueChange(e.target.value)}
        sx={{
          height: "100%",
          border: 0,
          background: "none",
          width: "100%",
          outline: 0,
          "&::placeholder": {
            color: (theme) => theme.palette.text.secondary,
          },
        }}
      />
    </Box>
  );
});
