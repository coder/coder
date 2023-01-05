import Divider from "@material-ui/core/Divider"
import MenuItem from "@material-ui/core/MenuItem"
import { makeStyles } from "@material-ui/core/styles"
import AccountIcon from "@material-ui/icons/AccountCircleOutlined"
import BugIcon from "@material-ui/icons/BugReportOutlined"
import ChatIcon from "@material-ui/icons/ChatOutlined"
import LaunchIcon from "@material-ui/icons/LaunchOutlined"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { Link } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import DocsIcon from "@material-ui/icons/MenuBook"
import LogoutIcon from "@material-ui/icons/ExitToAppOutlined"
import { combineClasses } from "util/combineClasses"

export const Language = {
  accountLabel: "Account",
  docsLabel: "Documentation",
  signOutLabel: "Sign Out",
  bugLabel: "Report a Bug",
  discordLabel: "Join the Coder Discord",
  copyrightText: `\u00a9 ${new Date().getFullYear()} Coder Technologies, Inc.`,
}

export interface UserDropdownContentProps {
  user: TypesGen.User
  buildInfo?: TypesGen.BuildInfoResponse
  onPopoverClose: () => void
  onSignOut: () => void
}

export const UserDropdownContent: FC<UserDropdownContentProps> = ({
  buildInfo,
  user,
  onPopoverClose,
  onSignOut,
}) => {
  const styles = useStyles()
  const githubUrl = `https://github.com/coder/coder/issues/new?labels=needs+grooming&body=${encodeURIComponent(`Version: [\`${buildInfo?.version}\`](${buildInfo?.external_url})

  <!--- Ask a question or leave feedback! -->`)}`
  const discordUrl = `https://coder.com/chat?utm_source=coder&utm_medium=coder&utm_campaign=server-footer`

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

      <a
        href="https://coder.com/docs/coder-oss"
        target="_blank"
        rel="noreferrer"
        className={styles.link}
      >
        <MenuItem className={styles.menuItem} onClick={onPopoverClose}>
          <DocsIcon className={styles.menuItemIcon} />
          <span className={styles.menuItemText}>{Language.docsLabel}</span>
        </MenuItem>
      </a>

      <a
        href={githubUrl}
        target="_blank"
        rel="noreferrer"
        className={styles.link}
      >
        <MenuItem className={styles.menuItem} onClick={onPopoverClose}>
          <BugIcon className={styles.menuItemIcon} />
          <span className={styles.menuItemText}>{Language.bugLabel}</span>
        </MenuItem>
      </a>

      <a
        href={discordUrl}
        target="_blank"
        rel="noreferrer"
        className={styles.link}
      >
        <MenuItem className={styles.menuItem} onClick={onPopoverClose}>
          <ChatIcon className={styles.menuItemIcon} />
          <span className={styles.menuItemText}>{Language.discordLabel}</span>
        </MenuItem>
      </a>

      <Divider className={styles.divider} />

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
