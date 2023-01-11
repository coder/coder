import Badge from "@material-ui/core/Badge"
import MenuItem from "@material-ui/core/MenuItem"
import { makeStyles } from "@material-ui/core/styles"
import { useState, FC, PropsWithChildren, MouseEvent } from "react"
import { colors } from "theme/colors"
import * as TypesGen from "../../api/typesGenerated"
import { navHeight } from "../../theme/constants"
import { BorderedMenu } from "../BorderedMenu/BorderedMenu"
import { CloseDropdown, OpenDropdown } from "../DropdownArrows/DropdownArrows"
import { UserAvatar } from "../UserAvatar/UserAvatar"
import { UserDropdownContent } from "../UserDropdownContent/UserDropdownContent"

export interface UserDropdownProps {
  user: TypesGen.User
  buildInfo?: TypesGen.BuildInfoResponse
  onSignOut: () => void
}

export const UserDropdown: FC<PropsWithChildren<UserDropdownProps>> = ({
  buildInfo,
  user,
  onSignOut,
}: UserDropdownProps) => {
  const styles = useStyles()
  const [anchorEl, setAnchorEl] = useState<HTMLElement | undefined>()

  const handleDropdownClick = (ev: MouseEvent<HTMLLIElement>): void => {
    setAnchorEl(ev.currentTarget)
  }
  const onPopoverClose = () => {
    setAnchorEl(undefined)
  }

  return (
    <>
      <MenuItem
        className={styles.menuItem}
        onClick={handleDropdownClick}
        data-testid="user-dropdown-trigger"
      >
        <div className={styles.inner}>
          <Badge overlap="circular">
            <UserAvatar username={user.username} avatarURL={user.avatar_url} />
          </Badge>
          {anchorEl ? (
            <CloseDropdown color={colors.gray[6]} />
          ) : (
            <OpenDropdown color={colors.gray[6]} />
          )}
        </div>
      </MenuItem>

      <BorderedMenu
        anchorEl={anchorEl}
        getContentAnchorEl={null}
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
          onPopoverClose={onPopoverClose}
          onSignOut={onSignOut}
        />
      </BorderedMenu>
    </>
  )
}

export const useStyles = makeStyles((theme) => ({
  divider: {
    marginTop: theme.spacing(1),
    marginBottom: theme.spacing(1),
  },
  inner: {
    display: "flex",
    alignItems: "center",
    minWidth: 0,
    maxWidth: 300,
  },
  menuItem: {
    height: navHeight,
    padding: theme.spacing(1.5, 0),

    "&:hover": {
      backgroundColor: "transparent",
    },
  },
}))
