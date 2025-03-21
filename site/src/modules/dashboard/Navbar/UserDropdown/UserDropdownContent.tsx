import {
	type CSSObject,
	type Interpolation,
	type Theme,
	css,
} from "@emotion/react";
import AccountIcon from "@mui/icons-material/AccountCircleOutlined";
import BugIcon from "@mui/icons-material/BugReportOutlined";
import ChatIcon from "@mui/icons-material/ChatOutlined";
import LogoutIcon from "@mui/icons-material/ExitToAppOutlined";
import InstallDesktopIcon from "@mui/icons-material/InstallDesktop";
import LaunchIcon from "@mui/icons-material/LaunchOutlined";
import DocsIcon from "@mui/icons-material/MenuBook";
import Divider from "@mui/material/Divider";
import MenuItem from "@mui/material/MenuItem";
import type { SvgIconProps } from "@mui/material/SvgIcon";
import Tooltip from "@mui/material/Tooltip";
import type * as TypesGen from "api/typesGenerated";
import { CopyButton } from "components/CopyButton/CopyButton";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Stack } from "components/Stack/Stack";
import { usePopover } from "components/deprecated/Popover/Popover";
import type { FC } from "react";
import { Link } from "react-router-dom";

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
			case "star":
				return <GithubStar css={styles.menuItemIcon} />;
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

			<Link to="/install" css={styles.link}>
				<MenuItem css={styles.menuItem} onClick={onPopoverClose}>
					<InstallDesktopIcon css={styles.menuItemIcon} />
					<span css={styles.menuItemText}>Install CLI</span>
				</MenuItem>
			</Link>

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
							href={link.target}
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

				{buildInfo?.deployment_id && (
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
								{buildInfo.deployment_id}
							</div>
						</Tooltip>
						<CopyButton
							text={buildInfo.deployment_id}
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

export const GithubStar: FC<SvgIconProps> = (props) => (
	<svg
		aria-hidden="true"
		height="16"
		viewBox="0 0 16 16"
		version="1.1"
		width="16"
		data-view-component="true"
		fill="currentColor"
		{...props}
	>
		<path d="M8 .25a.75.75 0 0 1 .673.418l1.882 3.815 4.21.612a.75.75 0 0 1 .416 1.279l-3.046 2.97.719 4.192a.751.751 0 0 1-1.088.791L8 12.347l-3.766 1.98a.75.75 0 0 1-1.088-.79l.72-4.194L.818 6.374a.75.75 0 0 1 .416-1.28l4.21-.611L7.327.668A.75.75 0 0 1 8 .25Z"></path>
	</svg>
);

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
