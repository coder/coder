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
import { cn } from "#/utils/cn";
import { SupportIcon } from "../SupportIcon";

// Mock AI spend data for the cost controls presentation.
// Add ?overspend to the URL to demo the over-90% red bar state.
const MOCK_AI_SPEND_NORMAL = { spentUSD: 819, limitUSD: 1200 };
const MOCK_AI_SPEND_OVER = { spentUSD: 1140, limitUSD: 1200 };

interface UserDropdownContentProps {
	user: TypesGen.User;
	buildInfo?: TypesGen.BuildInfoResponse;
	supportLinks: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
}

const AISpendBar: FC<{ spent: number; limit: number }> = ({
	spent,
	limit,
}) => {
	const pct = Math.min((spent / limit) * 100, 100);
	const isOver = pct >= 90;
	return (
		<div className="flex flex-col gap-1.5 px-3 py-2">
			<span
				className={cn(
					"text-xs",
					isOver ? "text-content-destructive" : "text-content-secondary",
				)}
			>
				AI spend - ${spent.toLocaleString("en-US")} / $
				{limit.toLocaleString("en-US")} USD
			</span>
			<div className="h-3 w-full rounded bg-surface-secondary overflow-hidden">
				<div
					className={cn(
						"h-full rounded transition-all",
						isOver ? "bg-highlight-red" : "bg-content-secondary",
					)}
					style={{ width: `${pct}%` }}
				/>
			</div>
		</div>
	);
};

export const UserDropdownContent: FC<UserDropdownContentProps> = ({
	user,
	buildInfo,
	supportLinks,
	onSignOut,
}) => {
	const { showCopiedSuccess, copyToClipboard } = useClipboard();
	const isOverSpendDemo = window.location.search.includes("overspend");
	const mockSpend = isOverSpendDemo ? MOCK_AI_SPEND_OVER : MOCK_AI_SPEND_NORMAL;

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

			<AISpendBar
				spent={mockSpend.spentUSD}
				limit={mockSpend.limitUSD}
			/>

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
