import { useState } from "react";
import { useQuery } from "react-query";
import { getErrorMessage } from "#/api/errors";
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
import { SeverityBadge, severityIndicators } from "./severityIndicators";
import { UserDropdownAISpend } from "./UserDropdownAISpend";
import { UserDropdownContent } from "./UserDropdownContent";
import { UserDropdownOrgSelect } from "./UserDropdownOrgSelect";
import { UserDropdownPremiumTrialCTA } from "./UserDropdownPremiumTrialCTA";
import { getSpendSeverity, getViewedSpend } from "./viewedSpend";

type UserDropdownProps = {
	user: TypesGen.User;
	buildInfo?: TypesGen.BuildInfoResponse;
	supportLinks: readonly TypesGen.LinkConfig[];
	codernautsEnabled?: boolean;
	onSignOut: () => void;
	canViewLicenses: boolean;
};

export const UserDropdown: React.FC<UserDropdownProps> = ({
	buildInfo,
	user,
	supportLinks,
	codernautsEnabled,
	onSignOut,
	canViewLicenses,
}) => {
	const aibridgeVisible = Boolean(useFeatureVisibility().aibridge);
	const { data, error } = useQuery({
		...meAISpend(),
		enabled: aibridgeVisible,
	});
	const [selectedOrgId, setSelectedOrgId] = useState<string>();

	// A failed poll keeps the last spend and the open selector on screen, with
	// the error beside them.
	const viewed =
		aibridgeVisible && data ? getViewedSpend(data, selectedOrgId) : undefined;
	const spend = viewed?.spend ?? null;
	const indicator = severityIndicators[getSpendSeverity(spend)];

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					className="relative bg-transparent cursor-pointer p-0"
				>
					<Avatar fallback={user.username} src={user.avatar_url} size="lg" />
					<span className="sr-only">
						{indicator ? `User menu. ${indicator.label}` : "User menu"}
					</span>
					{indicator && (
						<SeverityBadge
							indicator={indicator}
							className="absolute -top-2 -right-2"
						/>
					)}
				</button>
			</DropdownMenuTrigger>

			<DropdownMenuContent align="end" className="w-65">
				<UserDropdownContent
					user={user}
					buildInfo={buildInfo}
					profileExtra={
						data &&
						(viewed?.shownOrgSpend || spend) && (
							<>
								<DropdownMenuSeparator />
								{viewed?.shownOrgSpend && (
									<UserDropdownOrgSelect
										organizations={data.organizations}
										selected={viewed.shownOrgSpend.organization}
										onSelectedOrgIdChange={setSelectedOrgId}
									/>
								)}
								{spend ? (
									<UserDropdownAISpend
										{...spend}
										periodStart={data.period_start}
										periodEnd={data.period_end}
									/>
								) : (
									<div className="px-2 py-2 text-xs text-content-secondary">
										AI spend unavailable
									</div>
								)}
								{error && (
									<div className="px-2 pb-2 text-xs text-content-secondary">
										Couldn't refresh AI spend:{" "}
										{getErrorMessage(error, "Unknown error")}
									</div>
								)}
							</>
						)
					}
					supportLinks={supportLinks}
					codernautsEnabled={codernautsEnabled}
					onSignOut={onSignOut}
					trialCta={
						<UserDropdownPremiumTrialCTA canViewLicenses={canViewLicenses} />
					}
				/>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
