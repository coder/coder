import type * as TypesGen from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { CopyButton } from "components/CopyButton/CopyButton";
import {
	DropdownMenuItem,
	DropdownMenuSeparator,
} from "components/DropdownMenu/DropdownMenu";
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
	signOutLabel: "Sign Out",
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
		<>
			<DropdownMenuItem
				className="flex items-center gap-3 [&_img]:w-full [&_img]:h-full"
				asChild
			>
				<Link to="/settings/account">
					<Avatar fallback={user.username} src={user.avatar_url} size="lg" />
					<div className="flex flex-col">
						<span className="text-white">{user.username}</span>
						<span className="text-xs font-semibold">{user.email}</span>
					</div>
				</Link>
			</DropdownMenuItem>
			<DropdownMenuSeparator />
			<DropdownMenuItem asChild>
				<Link to="/install">
					<MonitorDownIcon />
					<span>Install CLI</span>
				</Link>
			</DropdownMenuItem>
			<DropdownMenuItem asChild>
				<Link to="/settings/account">
					<CircleUserIcon />
					<span>Account</span>
				</Link>
			</DropdownMenuItem>
			<DropdownMenuItem onClick={onSignOut}>
				<LogOutIcon />
				<span>Sign Out</span>
			</DropdownMenuItem>
			{supportLinks && supportLinks.length > 0 && (
				<>
					<DropdownMenuSeparator />
					{supportLinks.map((link) => (
						<DropdownMenuItem key={link.name} asChild>
							<a href={link.target} target="_blank" rel="noreferrer">
								{link.icon && <SupportIcon icon={link.icon} />}
								<span>{link.name}</span>
							</a>
						</DropdownMenuItem>
					))}
				</>
			)}
			<DropdownMenuSeparator />
			<Tooltip disableHoverableContent>
				<TooltipTrigger asChild>
					<DropdownMenuItem className="text-xs" asChild>
						<a
							href={buildInfo?.external_url}
							className="flex items-center gap-2"
							target="_blank"
							rel="noreferrer"
						>
							<span className="flex-1">{buildInfo?.version}</span>
							<SquareArrowOutUpRightIcon className="!size-icon-xs" />
						</a>
					</DropdownMenuItem>
				</TooltipTrigger>
				<TooltipContent side="bottom">Browse the source code</TooltipContent>
			</Tooltip>
			{buildInfo?.deployment_id && (
				<Tooltip disableHoverableContent>
					<TooltipTrigger asChild>
						<DropdownMenuItem
							className="text-xs"
							onClick={(e) => {
								e.preventDefault();
							}}
							asChild
						>
							<div className="flex items-center gap-2 relative">
								<span className="truncate">{buildInfo.deployment_id}</span>
								<CopyButton
									className="size-4 after:content-[''] after:absolute after:inset-0"
									text={buildInfo.deployment_id}
									label="Copy deployment ID"
								/>
							</div>
						</DropdownMenuItem>
					</TooltipTrigger>
					<TooltipContent side="bottom">Deployment Identifier</TooltipContent>
				</Tooltip>
			)}
			<DropdownMenuItem className="text-xs" disabled>
				<span>{Language.copyrightText}</span>
			</DropdownMenuItem>
		</>
	);
};
