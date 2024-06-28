import {
  css,
  type CSSObject,
  type Interpolation,
  type Theme,
} from "@emotion/react";
import AccountIcon from "@mui/icons-material/AccountCircleOutlined";
import BugIcon from "@mui/icons-material/BugReportOutlined";
import ChatIcon from "@mui/icons-material/ChatOutlined";
import LogoutIcon from "@mui/icons-material/ExitToAppOutlined";
import LaunchIcon from "@mui/icons-material/LaunchOutlined";
import DocsIcon from "@mui/icons-material/MenuBook";
import Divider from "@mui/material/Divider";
import MenuItem from "@mui/material/MenuItem";
import Tooltip from "@mui/material/Tooltip";
import type { FC } from "react";
import { Link } from "react-router-dom";
import type * as TypesGen from "api/typesGenerated";
import { CopyButton } from "components/CopyButton/CopyButton";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { usePopover } from "components/Popover/Popover";
import { Stack } from "components/Stack/Stack";

export const Language = {
  accountLabel: "Account",
  signOutLabel: "Sign Out",
  copyrightText: `\u00a9 ${new Date().getFullYear()} Coder Technologies, Inc.`,
};

export interface UserDropdownContentProps {
  user: TypesGen.User;
  buildInfo?: TypesGen.BuildInfoResponse;
  supportLinks?: readonly TypesGen.LinkConfig[];
  onSignOut: () => void;
}

export const UserDropdownContent: FC<UserDropdownContentProps> = ({
  user,
  buildInfo,
  supportLinks,
  onSignOut,
}) => {
  const popover = usePopover();

  const onPopoverClose = () => {
    popover.setOpen(false);
  };

  const renderMenuIcon = (icon: string): JSX.Element => {
    switch (icon) {
      case "bug":
        return <BugIcon css={styles.menuItemIcon} />;
      case "chat":
        return <ChatIcon css={styles.menuItemIcon} />;
      case "docs":
        return <DocsIcon css={styles.menuItemIcon} />;
      default:
        return (
          <ExternalImage
            src={icon}
            css={{ maxWidth: "20px", maxHeight: "20px" }}
          />
        );
    }
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
                {renderMenuIcon(link.icon)}
                <span css={styles.menuItemText}>{link.name}</span>
              </MenuItem>
            </a>
          ))}
        </>
      )}

      <Divider css={{ marginBottom: "0 !important" }} />

      <Stack css={styles.info} spacing={0}>
        <Tooltip title="Browse the source code">
          <a
            css={[styles.footerText, styles.buildInfo]}
            href={buildInfo?.external_url}
            target="_blank"
            rel="noreferrer"
          >
            {buildInfo?.version} <LaunchIcon />
          </a>
        </Tooltip>

        {Boolean(buildInfo?.deployment_id) && (
          <div
            css={css`
              font-size: 12px;
              display: flex;
              align-items: center;
            `}
          >
            <Tooltip title="Deployment Identifier">
              <div
                css={css`
                  white-space: nowrap;
                  overflow: hidden;
                  text-overflow: ellipsis;
                `}
              >
                {buildInfo?.deployment_id}
              </div>
            </Tooltip>
            <CopyButton
              text={buildInfo!.deployment_id}
              buttonStyles={css`
                width: 16px;
                height: 16px;

                svg {
                  width: 16px;
                  height: 16px;
                }
              `}
            />
          </div>
        )}

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

const styles = {
  info: (theme) => [
    theme.typography.body2 as CSSObject,
    {
      padding: 20,
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
    gap: 20px;
    padding: 8px 20px;

    &:hover {
      background-color: ${theme.palette.action.hover};
      transition: background-color 0.3s ease;
    }
  `,
  menuItemIcon: (theme) => ({
    color: theme.palette.text.secondary,
    width: 20,
    height: 20,
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
