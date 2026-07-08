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
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { getSeverity, severityBorderClassName } from "#/utils/budget";
import { UserDropdownAISpend } from "./UserDropdownAISpend";
import { UserDropdownContent } from "./UserDropdownContent";

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
	const { experiments } = useDashboard();
	// TODO(AIGOV-443): drop the experiment gate once cost control is stable.
	const aibridgeVisible =
		useFeatureVisibility().aibridge &&
		experiments.includes("ai-gateway-cost-control");
	const { data, isError } = useQuery({
		...meAISpend(),
		enabled: aibridgeVisible,
	});

	// A null limit is unlimited and still shown.
	const hasValidSpend =
		!!data &&
		data.current_spend_micros >= 0 &&
		(data.spend_limit_micros === null || data.spend_limit_micros >= 0);
	const spend =
		aibridgeVisible && !isError && hasValidSpend
			? {
					currentSpend: data.current_spend_micros,
					spendLimit: data.spend_limit_micros,
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
						className={spend ? severityBorderClassName(severity) : undefined}
					/>
				</button>
			</DropdownMenuTrigger>

			<DropdownMenuContent align="end" className="min-w-auto w-[260px]">
				<UserDropdownContent
					user={user}
					buildInfo={buildInfo}
					profileExtra={
						spend && (
							<UserDropdownAISpend
								{...spend}
								header={<DropdownMenuSeparator />}
							/>
						)
					}
					supportLinks={supportLinks}
					onSignOut={onSignOut}
				/>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
