import type { FC } from "react";
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

// The normal state keeps the standard avatar border. Elevated states use a
// thicker border plus a small notification-style dot so the change is
// perceivable without relying on color alone.
const severityIndicators: Partial<
	Record<UsageSeverity, { border: string; dot: string; label: string }>
> = {
	warning: {
		border: "border-2 border-content-warning",
		dot: "bg-content-warning",
		label: "AI spend is nearing its limit",
	},
	exceeded: {
		border: "border-2 border-content-destructive",
		dot: "bg-content-destructive",
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
							className="relative bg-transparent border-0 cursor-pointer p-0"
						>
							<Avatar
								fallback={user.username}
								src={user.avatar_url}
								size="lg"
								className={indicator?.border}
							/>
							{indicator && (
								<span
									className={cn(
										"absolute -top-0.5 -right-0.5 size-2.5 rounded-full",
										indicator.dot,
									)}
								>
									<span className="sr-only">{indicator.label}</span>
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
