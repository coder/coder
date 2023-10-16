import Badge from "@mui/material/Badge";
import MenuItem from "@mui/material/MenuItem";
import { useState, FC, PropsWithChildren, MouseEvent } from "react";
import { colors } from "theme/colors";
import * as TypesGen from "api/typesGenerated";
import { navHeight } from "theme/constants";
import { BorderedMenu } from "./BorderedMenu";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { UserDropdownContent } from "./UserDropdownContent";
import { BUTTON_SM_HEIGHT } from "theme/theme";
import { css } from "@emotion/react";

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
  const [anchorEl, setAnchorEl] = useState<HTMLElement | undefined>();

  const handleDropdownClick = (ev: MouseEvent<HTMLLIElement>): void => {
    setAnchorEl(ev.currentTarget);
  };
  const onPopoverClose = () => {
    setAnchorEl(undefined);
  };

  return (
    <>
      <MenuItem
        css={(theme) => css`
          height: ${navHeight}px;
          padding: ${theme.spacing(1.5, 0)};

          &:hover {
            background-color: transparent;
          }
        `}
        onClick={handleDropdownClick}
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
          <DropdownArrow color={colors.gray[6]} close={Boolean(anchorEl)} />
        </div>
      </MenuItem>

      <BorderedMenu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "right",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "right",
        }}
        marginThreshold={0}
        variant="user-dropdown"
        onClose={onPopoverClose}
      >
        <UserDropdownContent
          user={user}
          buildInfo={buildInfo}
          supportLinks={supportLinks}
          onPopoverClose={onPopoverClose}
          onSignOut={onSignOut}
        />
      </BorderedMenu>
    </>
  );
};
