import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import Badge from "@mui/material/Badge";
import { useState, type FC } from "react";
import type * as TypesGen from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { BUTTON_SM_HEIGHT, navHeight } from "theme/constants";
import { UserDropdownContent } from "./UserDropdownContent";

export interface UserDropdownProps {
  user: TypesGen.User;
  buildInfo?: TypesGen.BuildInfoResponse;
  supportLinks?: readonly TypesGen.LinkConfig[];
  onSignOut: () => void;
}

export const UserDropdown: FC<UserDropdownProps> = ({
  buildInfo,
  user,
  supportLinks,
  onSignOut,
}) => {
  const theme = useTheme();
  const [open, setOpen] = useState(false);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger>
        <button css={styles.button} data-testid="user-dropdown-trigger">
          <div css={styles.badgeContainer}>
            <Badge overlap="circular">
              <UserAvatar
                css={styles.avatar}
                username={user.username}
                avatarURL={user.avatar_url}
              />
            </Badge>
            <DropdownArrow
              color={theme.experimental.l2.fill.solid}
              close={open}
            />
          </div>
        </button>
      </PopoverTrigger>

      <PopoverContent
        horizontal="right"
        css={{
          ".MuiPaper-root": {
            minWidth: "auto",
            width: 260,
            boxShadow: theme.shadows[6],
          },
        }}
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

const styles = {
  button: css`
    background: none;
    border: 0;
    cursor: pointer;
    height: ${navHeight}px;
    padding: 12px 0;

    &:hover {
      background-color: transparent;
    }
  `,

  badgeContainer: {
    display: "flex",
    alignItems: "center",
    minWidth: 0,
    maxWidth: 300,
  },

  avatar: {
    width: BUTTON_SM_HEIGHT,
    height: BUTTON_SM_HEIGHT,
    fontSize: 16,
  },
} satisfies Record<string, Interpolation<Theme>>;
