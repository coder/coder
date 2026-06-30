import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Switch } from "#/components/Switch/Switch";

interface ChatGoalSettingsProps {
	goalsEnabledData: TypesGen.ChatGoalsEnabledResponse | undefined;
	isLoadingGoalsEnabled: boolean;
	onSaveGoalsEnabled: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatGoalsEnabledRequest,
		unknown
	>;
	isSavingGoalsEnabled: boolean;
	isSaveGoalsEnabledError: boolean;
}

export const ChatGoalSettings: FC<ChatGoalSettingsProps> = ({
	goalsEnabledData,
	isLoadingGoalsEnabled,
	onSaveGoalsEnabled,
	isSavingGoalsEnabled,
	isSaveGoalsEnabledError,
}) => {
	const goalsEnabled = goalsEnabledData?.enabled ?? false;

	return (
		<div className="space-y-2">
			<div className="flex items-center justify-between gap-4">
				<div className="flex items-center gap-2">
					<h3 className="m-0 text-sm font-semibold text-content-primary">
						Chat goals
					</h3>
					<Badge size="sm" variant="warning" className="cursor-default">
						<TriangleAlertIcon className="size-3" />
						Experimental feature
					</Badge>
				</div>
				<div className="flex items-center gap-2">
					{isLoadingGoalsEnabled ? (
						<Skeleton className="h-5 w-10 rounded-full" aria-hidden="true" />
					) : (
						<Switch
							checked={goalsEnabled}
							onCheckedChange={(checked) =>
								onSaveGoalsEnabled({ enabled: checked })
							}
							aria-label="Enable chat goals"
							disabled={isSavingGoalsEnabled || isLoadingGoalsEnabled}
						/>
					)}
				</div>
			</div>
			<p className="m-0 text-xs text-content-secondary">
				Allow users to create and manage durable goals from the agent composer.
			</p>
			{isSaveGoalsEnabledError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save chat goals setting.
				</p>
			)}
		</div>
	);
};
