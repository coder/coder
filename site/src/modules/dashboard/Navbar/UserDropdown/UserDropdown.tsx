import { OctagonAlertIcon, TriangleAlertIcon } from "lucide-react";
import type { FC, JSX } from "react";
import { useQuery } from "react-query";
import { meAISpend } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { getSeverity, type UsageSeverity } from "#/utils/budget";
import { cn } from "#/utils/cn";
import { UserDropdownAISpend } from "./UserDropdownAISpend";
import { UserDropdownContent } from "./UserDropdownContent";

// Elevated states show a corner badge with a distinct icon per state.
const severityIndicators: Partial<
	Record<UsageSeverity, { badge: string; icon: JSX.Element; label: string }>
> = {
	warning: {
		badge: "bg-surface-orange text-highlight-orange",
		icon: <TriangleAlertIcon aria-hidden className="size-3" />,
		label: "AI spend is nearing its limit",
	},
	exceeded: {
		badge: "bg-surface-red text-highlight-red",
		icon: <OctagonAlertIcon aria-hidden className="size-3" />,
		label: "AI spend limit exceeded",
	},
};

interface UserDropdownProps {
	user: TypesGen.User;
	buildInfo?: TypesGen.BuildInfoResponse;
	supportLinks: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
}

export const UserDropdown: FC<UserDropdownProps> = ({
	buildInfo,
	user,
	supportLinks,
	onSignOut,
}) => {
	const aibridgeVisible = Boolean(useFeatureVisibility().aibridge);
	const { data, isError } = useQuery({
		...meAISpend(),
		enabled: aibridgeVisible,
	});

	// A null budget is unlimited and still shown.
	const hasValidSpend =
		data !== undefined &&
		data.current_spend_micros >= 0 &&
		(data.effective_budget === null ||
			data.effective_budget.spend_limit_micros >= 0);
	const spend =
		aibridgeVisible && !isError && hasValidSpend
			? {
					currentSpend: data.current_spend_micros,
					spendLimit: data.effective_budget?.spend_limit_micros ?? null,
					periodStart: data.period_start,
					periodEnd: data.period_end,
				}
			: null;
	const severity =
		spend && spend.spendLimit !== null
			? getSeverity(spend.currentSpend, spend.spendLimit)
			: "normal";
	const indicator = spend ? severityIndicators[severity] : undefined;

	return (
		<DropdownMenu>
			<Tooltip>
				<DropdownMenuTrigger asChild>
					<TooltipTrigger asChild>
						<button
							type="button"
							aria-label={
								indicator ? `User menu. ${indicator.label}` : "User menu"
							}
							className="relative bg-transparent border-0 cursor-pointer p-0"
						>
							<Avatar
								fallback={user.username}
								src={user.avatar_url}
								size="lg"
							/>
							{indicator && (
								<span
									className={cn(
										"absolute -top-2 -right-2 flex size-[18px] items-center",
										"justify-center rounded",
										indicator.badge,
									)}
								>
									{indicator.icon}
								</span>
							)}
						</button>
					</TooltipTrigger>
				</DropdownMenuTrigger>
				{indicator && <TooltipContent>{indicator.label}</TooltipContent>}
			</Tooltip>

			<DropdownMenuContent align="end" className="min-w-auto w-[260px]">
				<UserDropdownContent
					user={user}
					buildInfo={buildInfo}
					profileExtra={
						spend && (
							<>
								<DropdownMenuSeparator />
								<UserDropdownAISpend {...spend} />
							</>
						)
					}
					supportLinks={supportLinks}
					onSignOut={onSignOut}
				/>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
