import Badge from "@mui/material/Badge";
import { type FC, type PropsWithChildren } from "react";
import { colors } from "theme/colors";
import type * as TypesGen from "api/typesGenerated";
import { BUTTON_SM_HEIGHT, navHeight } from "theme/constants";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { UserDropdownContent } from "./UserDropdownContent";
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
  isDefaultOpen?: boolean;
}

export const UserDropdown: FC<PropsWithChildren<UserDropdownProps>> = ({
  buildInfo,
  user,
  supportLinks,
  onSignOut,
  isDefaultOpen,
}: UserDropdownProps) => {
  return (
    <Popover isDefaultOpen={isDefaultOpen}>
      {(popover) => (
        <>
          <PopoverTrigger>
            <button
              css={css`
                background: none;
                border: 0;
                cursor: pointer;
                height: ${navHeight}px;
                padding: 12px 0;

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
                <DropdownArrow color={colors.gray[6]} close={popover.isOpen} />
              </div>
            </button>
          </PopoverTrigger>

          <PopoverContent
            horizontal="right"
            css={(theme) => ({
              ".MuiPaper-root": {
                minWidth: "auto",
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
        </>
      )}
    </Popover>
  );
};
