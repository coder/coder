import Badge from "@mui/material/Badge";
import MenuItem from "@mui/material/MenuItem";
import { FC, PropsWithChildren } from "react";
import { colors } from "theme/colors";
import * as TypesGen from "api/typesGenerated";
import { navHeight } from "theme/constants";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { UserDropdownContent } from "./UserDropdownContent";
import { BUTTON_SM_HEIGHT } from "theme/theme";
import { css } from "@emotion/react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";

export interface UserDropdownProps {
  user: TypesGen.User;
  buildInfo?: TypesGen.BuildInfoResponse;
  supportLinks?: TypesGen.LinkConfig[];
  onSignOut: () => void;
}

export const UserDropdown: FC<PropsWithChildren<UserDropdownProps>> = ({
  buildInfo,
  user,
  supportLinks,
  onSignOut,
}: UserDropdownProps) => {
  return (
    <Popover>
      <PopoverTrigger>
        <MenuItem
          css={(theme) => css`
            height: ${navHeight}px;
            padding: ${theme.spacing(1.5, 0)};

            &:hover {
              background-color: transparent;
            }
          `}
          data-testid="user-dropdown-trigger"
        >
          <div
            css={{
              display: "flex",
              alignItems: "center",
              minWidth: 0,
              maxWidth: 300,
            }}
          >
            <Badge overlap="circular">
              <UserAvatar
                sx={{
                  width: BUTTON_SM_HEIGHT,
                  height: BUTTON_SM_HEIGHT,
                  fontSize: 16,
                }}
                username={user.username}
                avatarURL={user.avatar_url}
              />
            </Badge>
            <DropdownArrow color={colors.gray[6]} />
          </div>
        </MenuItem>
      </PopoverTrigger>

      <PopoverContent
        horizontal="right"
        css={(theme) => ({
          ".MuiPaper-root": {
            width: 260,
            boxShadow: theme.shadows[6],
          },
        })}
      >
        <UserDropdownContent
          user={user}
          buildInfo={buildInfo}
          supportLinks={supportLinks}
          onSignOut={onSignOut}
        />
      </PopoverContent>
    </Popover>
  );
};
