import Divider from "@mui/material/Divider"
import MenuItem from "@mui/material/MenuItem"
import { makeStyles } from "@mui/styles"
import AccountIcon from "@mui/icons-material/AccountCircleOutlined"
import BugIcon from "@mui/icons-material/BugReportOutlined"
import ChatIcon from "@mui/icons-material/ChatOutlined"
import LaunchIcon from "@mui/icons-material/LaunchOutlined"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { Link } from "react-router-dom"
import * as TypesGen from "../../../../../api/typesGenerated"
import DocsIcon from "@mui/icons-material/MenuBook"
import LogoutIcon from "@mui/icons-material/ExitToAppOutlined"
import { combineClasses } from "utils/combineClasses"

export const Language = {
  accountLabel: "Account",
  signOutLabel: "Sign Out",
  copyrightText: `\u00a9 ${new Date().getFullYear()} Coder Technologies, Inc.`,
}

export interface UserDropdownContentProps {
  user: TypesGen.User
  buildInfo?: TypesGen.BuildInfoResponse
  supportLinks?: TypesGen.LinkConfig[]
  onPopoverClose: () => void
  onSignOut: () => void
}

export const UserDropdownContent: FC<UserDropdownContentProps> = ({
  buildInfo,
  user,
  supportLinks,
  onPopoverClose,
  onSignOut,
}) => {
  const styles = useStyles()

  return (
    <div>
      <Stack className={styles.info} spacing={0}>
        <span className={styles.userName}>{user.username}</span>
        <span className={styles.userEmail}>{user.email}</span>
      </Stack>

      <Divider className={styles.divider} />

      <Link to="/settings/account" className={styles.link}>
        <MenuItem className={styles.menuItem} onClick={onPopoverClose}>
          <AccountIcon className={styles.menuItemIcon} />
          <span className={styles.menuItemText}>{Language.accountLabel}</span>
        </MenuItem>
      </Link>

      <MenuItem className={styles.menuItem} onClick={onSignOut}>
        <LogoutIcon className={styles.menuItemIcon} />
        <span className={styles.menuItemText}>{Language.signOutLabel}</span>
      </MenuItem>

      <Divider className={styles.divider} />

      <>
        {supportLinks &&
          supportLinks.map((link) => (
            <a
              href={includeBuildInfo(link.target, buildInfo)}
              key={link.name}
              target="_blank"
              rel="noreferrer"
              className={styles.link}
            >
              <MenuItem className={styles.menuItem} onClick={onPopoverClose}>
                {link.icon === "bug" && (
                  <BugIcon className={styles.menuItemIcon} />
                )}
                {link.icon === "chat" && (
                  <ChatIcon className={styles.menuItemIcon} />
                )}
                {link.icon === "docs" && (
                  <DocsIcon className={styles.menuItemIcon} />
                )}
                <span className={styles.menuItemText}>{link.name}</span>
              </MenuItem>
            </a>
          ))}
      </>

      {supportLinks && <Divider className={styles.divider} />}

      <Stack className={styles.info} spacing={0}>
        <a
          title="Browse Source Code"
          className={combineClasses([styles.footerText, styles.buildInfo])}
          href={buildInfo?.external_url}
          target="_blank"
          rel="noreferrer"
        >
          {buildInfo?.version} <LaunchIcon />
        </a>

        <div className={styles.footerText}>{Language.copyrightText}</div>
      </Stack>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  info: {
    padding: theme.spacing(2.5),
    ...theme.typography.body2,
  },
  userName: {
    fontWeight: 600,
  },
  userEmail: {
    color: theme.palette.text.secondary,
    width: "100%",
    textOverflow: "ellipsis",
    overflow: "hidden",
  },
  link: {
    textDecoration: "none",
    color: "inherit",
  },
  menuItem: {
    gap: theme.spacing(2.5),
    padding: theme.spacing(1, 2.5),

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
      transition: "background-color 0.3s ease",
    },
  },
  menuItemIcon: {
    color: theme.palette.text.secondary,
    width: theme.spacing(2.5),
    height: theme.spacing(2.5),
  },
  menuItemText: {
    fontSize: 14,
  },
  divider: {
    margin: theme.spacing(1, 0),

    "&:first-of-type": {
      marginTop: 0,
    },

    "&:last-of-type": {
      marginBottom: 0,
    },
  },
  footerText: {
    fontSize: 12,
    textDecoration: "none",
    color: theme.palette.text.secondary,
    display: "flex",
    alignItems: "center",
    gap: 4,

    "& svg": {
      width: 12,
      height: 12,
    },
  },
  buildInfo: {
    color: theme.palette.text.primary,
  },
}))

const includeBuildInfo = (
  href: string,
  buildInfo?: TypesGen.BuildInfoResponse,
): string => {
  return href.replace(
    "{CODER_BUILD_INFO}",
    `${encodeURIComponent(
      `Version: [\`${buildInfo?.version}\`](${buildInfo?.external_url})`,
    )}`,
  )
}
