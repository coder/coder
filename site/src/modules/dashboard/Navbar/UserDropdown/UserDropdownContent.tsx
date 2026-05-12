import {
	CircleUserIcon,
	CopyIcon,
	LogOutIcon,
	MonitorDownIcon,
	SquareArrowOutUpRightIcon,
} from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { CheckIcon } from "#/components/AnimatedIcons/Check";
import {
	DropdownMenuItem,
	DropdownMenuSeparator,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useClipboard } from "#/hooks/useClipboard";
import { SupportIcon } from "../SupportIcon";

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
	const { showCopiedSuccess, copyToClipboard } = useClipboard();

	return (
		<>
			<DropdownMenuItem
				className="flex items-center gap-3 [&_img]:w-full [&_img]:h-full"
				asChild
			>
				<Link to="/settings/account">
					<div className="flex flex-col">
						<span className="text-content-primary">{user.username}</span>
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
			<DropdownMenuItem asChild>
				<Link to="/coder-cup">
					<svg
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						strokeWidth="1.5"
						strokeLinecap="round"
						strokeLinejoin="round"
						xmlns="http://www.w3.org/2000/svg"
					>
						<path d="M7,10 L5,15 L19,15 L17,10 Z" />
						<path d="M8,10 L9,7 L11,5 L13,5 L15,7 L16,10" />
						<line x1="6" y1="15" x2="4" y2="19" />
						<line x1="2" y1="19" x2="6" y2="19" />
						<line x1="18" y1="15" x2="20" y2="19" />
						<line x1="18" y1="19" x2="22" y2="19" />
						<path d="M10,15 L10.5,18 L13.5,18 L14,15" />
					</svg>
					<span>Codernauts</span>
				</Link>
			</DropdownMenuItem>{" "}
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
							onSelect={(e) => {
								e.preventDefault();
								copyToClipboard(buildInfo.deployment_id);
							}}
						>
							<span className="truncate flex-1">{buildInfo.deployment_id}</span>
							{showCopiedSuccess ? (
								<CheckIcon className="!size-icon-xs ml-auto" />
							) : (
								<CopyIcon className="!size-icon-xs ml-auto" />
							)}
						</DropdownMenuItem>
					</TooltipTrigger>
					<TooltipContent side="bottom">
						{showCopiedSuccess ? "Copied!" : "Copy deployment ID"}
					</TooltipContent>
				</Tooltip>
			)}
			<DropdownMenuItem className="text-xs" disabled>
				<span>&copy; {new Date().getFullYear()} Coder Technologies, Inc.</span>
			</DropdownMenuItem>
		</>
	);
};
