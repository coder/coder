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
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { getSeverity, type UsageSeverity } from "#/utils/budget";
import { UserDropdownAISpend } from "./UserDropdownAISpend";
import { UserDropdownContent } from "./UserDropdownContent";

const severityBorderClasses = {
	normal: "border-content-secondary",
	warning: "border-content-warning",
	exceeded: "border-content-destructive",
} as const satisfies Record<UsageSeverity, string>;

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

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					className="bg-transparent border-0 cursor-pointer p-0"
				>
					<Avatar
						fallback={user.username}
						src={user.avatar_url}
						size="lg"
						className={spend ? severityBorderClasses[severity] : undefined}
					/>
				</button>
			</DropdownMenuTrigger>

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
