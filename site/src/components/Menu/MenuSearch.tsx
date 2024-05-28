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
      onKeyDown={(e) => {
        if (e.key === "ArrowDown") {
          e.preventDefault();

          const popoverContent =
            e.currentTarget.closest<HTMLDivElement>(".MuiPaper-root");

          if (!popoverContent) {
            return;
          }

          const firstMenuItem =
            popoverContent.querySelector<HTMLElement>("[role=menuitem]");

          if (firstMenuItem) {
            firstMenuItem.focus();
          }
        }
      }}
      className={css({
        position: "sticky",
        top: 0,
        backgroundColor: theme.palette.background.paper,
        zIndex: 1,
        "& fieldset": {
          border: 0,
          borderRadius: 0,
          borderBottomStyle: "solid",
          borderBottomWidth: `1px !important`,
          borderColor: `${theme.palette.divider} !important`,
        },
      })}
    />
  );
};
