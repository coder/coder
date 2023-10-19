import Divider from "@mui/material/Divider";
import MenuItem from "@mui/material/MenuItem";
import AccountIcon from "@mui/icons-material/AccountCircleOutlined";
import BugIcon from "@mui/icons-material/BugReportOutlined";
import ChatIcon from "@mui/icons-material/ChatOutlined";
import LaunchIcon from "@mui/icons-material/LaunchOutlined";
import { Stack } from "components/Stack/Stack";
import { FC } from "react";
import { Link } from "react-router-dom";
import * as TypesGen from "api/typesGenerated";
import DocsIcon from "@mui/icons-material/MenuBook";
import LogoutIcon from "@mui/icons-material/ExitToAppOutlined";
import {
  css,
  type CSSObject,
  type Interpolation,
  type Theme,
} from "@emotion/react";
import { usePopover } from "components/Popover/Popover";

export const Language = {
  accountLabel: "Account",
  signOutLabel: "Sign Out",
  copyrightText: `\u00a9 ${new Date().getFullYear()} Coder Technologies, Inc.`,
};

const styles = {
  info: (theme) => [
    theme.typography.body2 as CSSObject,
    {
      padding: theme.spacing(2.5),
    },
  ],
  userName: {
    fontWeight: 600,
  },
  userEmail: (theme) => ({
    color: theme.palette.text.secondary,
    width: "100%",
    textOverflow: "ellipsis",
    overflow: "hidden",
  }),
  link: {
    textDecoration: "none",
    color: "inherit",
  },
  menuItem: (theme) => css`
    gap: ${theme.spacing(2.5)};
    padding: ${theme.spacing(1, 2.5)};

    &:hover {
      background-color: ${theme.palette.action.hover};
      transition: background-color 0.3s ease;
    }
  `,
  menuItemIcon: (theme) => ({
    color: theme.palette.text.secondary,
    width: theme.spacing(2.5),
    height: theme.spacing(2.5),
  }),
  menuItemText: {
    fontSize: 14,
  },
  footerText: (theme) => css`
    font-size: 12px;
    text-decoration: none;
    color: ${theme.palette.text.secondary};
    display: flex;
    align-items: center;
    gap: 4px;

    & svg {
      width: 12px;
      height: 12px;
    }
  `,
  buildInfo: (theme) => ({
    color: theme.palette.text.primary,
  }),
} satisfies Record<string, Interpolation<Theme>>;

export interface UserDropdownContentProps {
  user: TypesGen.User;
  buildInfo?: TypesGen.BuildInfoResponse;
  supportLinks?: TypesGen.LinkConfig[];
  onSignOut: () => void;
}

export const UserDropdownContent: FC<UserDropdownContentProps> = ({
  buildInfo,
  user,
  supportLinks,
  onSignOut,
}) => {
  const popover = usePopover();

  const onPopoverClose = () => {
    popover.setIsOpen(false);
  };

  return (
    <div>
      <Stack css={styles.info} spacing={0}>
        <span css={styles.userName}>{user.username}</span>
        <span css={styles.userEmail}>{user.email}</span>
      </Stack>

      <Divider css={{ marginBottom: 8 }} />

      <Link to="/settings/account" css={styles.link}>
        <MenuItem css={styles.menuItem} onClick={onPopoverClose}>
          <AccountIcon css={styles.menuItemIcon} />
          <span css={styles.menuItemText}>{Language.accountLabel}</span>
        </MenuItem>
      </Link>

      <MenuItem css={styles.menuItem} onClick={onSignOut}>
        <LogoutIcon css={styles.menuItemIcon} />
        <span css={styles.menuItemText}>{Language.signOutLabel}</span>
      </MenuItem>

      {supportLinks && (
        <>
          <Divider />
          {supportLinks.map((link) => (
            <a
              href={includeBuildInfo(link.target, buildInfo)}
              key={link.name}
              target="_blank"
              rel="noreferrer"
              css={styles.link}
            >
              <MenuItem css={styles.menuItem} onClick={onPopoverClose}>
                {link.icon === "bug" && <BugIcon css={styles.menuItemIcon} />}
                {link.icon === "chat" && <ChatIcon css={styles.menuItemIcon} />}
                {link.icon === "docs" && <DocsIcon css={styles.menuItemIcon} />}
                <span css={styles.menuItemText}>{link.name}</span>
              </MenuItem>
            </a>
          ))}
        </>
      )}

      <Divider css={{ marginBottom: "0 !important" }} />

      <Stack css={styles.info} spacing={0}>
        <a
          title="Browse Source Code"
          css={[styles.footerText, styles.buildInfo]}
          href={buildInfo?.external_url}
          target="_blank"
          rel="noreferrer"
        >
          {buildInfo?.version} <LaunchIcon />
        </a>

        <div css={styles.footerText}>{Language.copyrightText}</div>
      </Stack>
    </div>
  );
};

const includeBuildInfo = (
  href: string,
  buildInfo?: TypesGen.BuildInfoResponse,
): string => {
  return href.replace(
    "{CODER_BUILD_INFO}",
    `${encodeURIComponent(
      `Version: [\`${buildInfo?.version}\`](${buildInfo?.external_url})`,
    )}`,
  );
};
