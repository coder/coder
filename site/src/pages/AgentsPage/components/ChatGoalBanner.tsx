import {
	CheckIcon,
	CirclePauseIcon,
	CirclePlayIcon,
	TargetIcon,
	Trash2Icon,
} from "lucide-react";
import type { ComponentProps, FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";

export type ChatGoalAction = Exclude<TypesGen.ChatGoalMutationAction, "set">;

type ChatGoalBannerProps = {
	goal: TypesGen.ChatGoal | undefined;
	isActionPending?: boolean;
	isActionDisabled?: boolean;
	onAction: (action: ChatGoalAction) => void;
};

const statusLabel = (status: TypesGen.ChatGoalStatus): string => {
	switch (status) {
		case "active":
			return "Active";
		case "paused":
			return "Paused";
		case "complete":
			return "Complete";
		case "cleared":
			return "Cleared";
		case "replaced":
			return "Replaced";
	}
};

const statusVariant = (
	status: TypesGen.ChatGoalStatus,
): ComponentProps<typeof Badge>["variant"] => {
	switch (status) {
		case "active":
			return "info";
		case "paused":
			return "warning";
		case "complete":
			return "green";
		case "cleared":
		case "replaced":
			return "default";
	}
};

const actionsForStatus = (
	status: TypesGen.ChatGoalStatus,
): ChatGoalAction[] => {
	switch (status) {
		case "active":
			return ["pause", "complete", "clear"];
		case "paused":
			return ["resume", "complete", "clear"];
		case "complete":
			return ["clear"];
		case "cleared":
		case "replaced":
			return [];
	}
};

const actionLabel = (action: ChatGoalAction): string => {
	switch (action) {
		case "pause":
			return "Pause";
		case "resume":
			return "Resume";
		case "complete":
			return "Complete";
		case "clear":
			return "Clear";
	}
};

const ActionIcon = ({ action }: { action: ChatGoalAction }) => {
	switch (action) {
		case "pause":
			return <CirclePauseIcon />;
		case "resume":
			return <CirclePlayIcon />;
		case "complete":
			return <CheckIcon />;
		case "clear":
			return <Trash2Icon />;
	}
};

const visibleGoalStatuses: ReadonlySet<TypesGen.ChatGoalStatus> = new Set([
	"active",
	"paused",
	"complete",
]);

export const ChatGoalBanner: FC<ChatGoalBannerProps> = ({
	goal,
	isActionPending = false,
	isActionDisabled = false,
	onAction,
}) => {
	if (!goal || !visibleGoalStatuses.has(goal.status)) {
		return null;
	}

	const actions = actionsForStatus(goal.status);
	const disabled = isActionPending || isActionDisabled;

	return (
		<section
			aria-label="Current goal"
			className="mx-auto mb-2 flex w-full max-w-3xl flex-col gap-2 rounded-lg border border-border-default bg-surface-secondary px-3 py-2 text-sm shadow-sm sm:flex-row sm:items-center sm:justify-between"
		>
			<div className="flex min-w-0 items-start gap-2">
				<TargetIcon className="mt-0.5 size-icon-sm shrink-0 text-content-secondary" />
				<div className="min-w-0 space-y-1">
					<div className="flex flex-wrap items-center gap-2">
						<span className="font-medium text-content-primary">Goal</span>
						<Badge size="sm" variant={statusVariant(goal.status)}>
							{statusLabel(goal.status)}
						</Badge>
					</div>
					<p className="whitespace-pre-wrap break-words text-content-secondary">
						{goal.objective.trim() || "No objective provided."}
					</p>
					{goal.completion_summary ? (
						<p className="whitespace-pre-wrap break-words text-xs text-content-secondary">
							Summary: {goal.completion_summary}
						</p>
					) : null}
				</div>
			</div>
			{actions.length > 0 ? (
				<div className="flex flex-wrap gap-1 sm:justify-end">
					{actions.map((action) => (
						<Button
							key={action}
							size="xs"
							variant={action === "clear" ? "subtle" : "outline"}
							disabled={disabled}
							onClick={() => onAction(action)}
						>
							<ActionIcon action={action} />
							{actionLabel(action)}
						</Button>
					))}
				</div>
			) : null}
		</section>
	);
};
