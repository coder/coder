import { css } from "@emotion/css";
import { useTheme } from "@emotion/react";
import type { FC } from "react";
import {
  SearchField,
  type SearchFieldProps,
} from "components/Search/SearchField";

export const MenuSearch: FC<SearchFieldProps> = (props) => {
  const theme = useTheme();

  return (
    <SearchField
      {...props}
      className={css({
        "& fieldset": {
          border: 0,
          borderRadius: 0,
          borderBottom: `1px solid ${theme.palette.divider}`,
        },
      })}
    />
  );
};
