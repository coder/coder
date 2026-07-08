import type { FC } from "react";
import { useQuery } from "react-query";
import type { UserAISpend } from "#/api/api";
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
import {
	getSeverity,
	severityBorderClassName,
	usageProgressPercentage,
} from "#/utils/budget";
import { type AISpend, UserDropdownAISpend } from "./UserDropdownAISpend";
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

	const spend = toAISpend(aibridgeVisible && !isError, data);

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
						className={
							spend ? severityBorderClassName(spend.severity) : undefined
						}
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
								spend={spend}
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

/** Resolves AI spend for the avatar border and dropdown section, or null when
 * it should be hidden. */
export function toAISpend(
	visible: boolean,
	data: UserAISpend | undefined,
): AISpend | null {
	if (!visible || !data) {
		return null;
	}

	const { current_spend_micros: currentSpend, spend_limit_micros: spendLimit } =
		data;

	// Hide on invalid spend data. A null limit means unlimited, which is shown.
	if (currentSpend < 0 || (spendLimit !== null && spendLimit < 0)) {
		return null;
	}

	return {
		currentSpend,
		spendLimit,
		percent:
			spendLimit === null
				? 0
				: usageProgressPercentage(currentSpend, spendLimit),
		severity:
			spendLimit === null ? "normal" : getSeverity(currentSpend, spendLimit),
	};
}
