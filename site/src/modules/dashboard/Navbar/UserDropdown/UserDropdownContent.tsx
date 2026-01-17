import {
	type CSSObject,
	css,
	type Interpolation,
	type Theme,
} from "@emotion/react";
import Divider from "@mui/material/Divider";
import MenuItem from "@mui/material/MenuItem";
import { PopoverClose } from "@radix-ui/react-popover";
import type * as TypesGen from "api/typesGenerated";
import { CopyButton } from "components/CopyButton/CopyButton";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	CircleUserIcon,
	LogOutIcon,
	MonitorDownIcon,
	SquareArrowOutUpRightIcon,
} from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import { SupportIcon } from "../SupportIcon";

export const Language = {
	accountLabel: "Account",
	signOutLabel: "Sign out",
	copyrightText: `\u00a9 ${new Date().getFullYear()} Coder Technologies, Inc.`,
};

interface UserDropdownContentProps {
	user: TypesGen.User;
	buildInfo?: TypesGen.BuildInfoResponse;
	supportLinks: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
}

export const UserDropdownContent: FC<UserDropdownContentProps> = ({
	user,
	buildInfo,
	supportLinks,
	onSignOut,
}) => {
	return (
		<div>
			<Stack css={styles.info} spacing={0}>
				<span css={styles.userName}>{user.username}</span>
				<span css={styles.userEmail}>{user.email}</span>
			</Stack>

			<Divider css={{ marginBottom: 8 }} />

			<Link to="/install" css={styles.link}>
				<PopoverClose asChild>
					<MenuItem css={styles.menuItem}>
						<MonitorDownIcon className="size-5 text-content-secondary" />
						<span css={styles.menuItemText}>Install CLI</span>
					</MenuItem>
				</PopoverClose>
			</Link>

			<Link to="/settings/account" css={styles.link}>
				<PopoverClose asChild>
					<MenuItem css={styles.menuItem}>
						<CircleUserIcon className="size-5 text-content-secondary" />
						<span css={styles.menuItemText}>{Language.accountLabel}</span>
					</MenuItem>
				</PopoverClose>
			</Link>

			<MenuItem css={styles.menuItem} onClick={onSignOut}>
				<LogOutIcon className="size-5 text-content-secondary" />
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
							<PopoverClose asChild>
								<MenuItem css={styles.menuItem}>
									{link.icon && (
										<SupportIcon
											icon={link.icon}
											className="size-5 text-content-secondary"
										/>
									)}
									<span css={styles.menuItemText}>{link.name}</span>
								</MenuItem>
							</PopoverClose>
						</a>
					))}
				</>
			)}

			<Divider css={{ marginBottom: "0 !important" }} />

			<Stack css={styles.info} spacing={0}>
				<Tooltip>
					<TooltipTrigger asChild>
						<a
							css={[styles.footerText, styles.buildInfo]}
							href={buildInfo?.external_url}
							target="_blank"
							rel="noreferrer"
						>
							{buildInfo?.version} <SquareArrowOutUpRightIcon />
						</a>
					</TooltipTrigger>
					<TooltipContent side="bottom">Browse the source code</TooltipContent>
				</Tooltip>

				{buildInfo?.deployment_id && (
					<div className="flex items-center text-xs">
						<Tooltip>
							<TooltipTrigger asChild>
								<span className="whitespace-nowrap overflow-hidden text-ellipsis">
									{buildInfo.deployment_id}
								</span>
							</TooltipTrigger>
							<TooltipContent side="bottom">
								Deployment Identifier
							</TooltipContent>
						</Tooltip>
						<CopyButton
							text={buildInfo.deployment_id}
							label="Copy deployment ID"
						/>
					</div>
				)}

				<div css={styles.footerText}>{Language.copyrightText}</div>
			</Stack>
		</div>
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
