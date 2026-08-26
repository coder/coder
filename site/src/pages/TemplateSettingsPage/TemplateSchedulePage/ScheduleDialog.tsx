import type { FC } from "react";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import type { ConfirmDialogProps } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	Dialog,
	DialogActions,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";

interface ScheduleDialogProps extends Pick<
	ConfirmDialogProps,
	| "open"
	| "onClose"
	| "onConfirm"
	| "title"
	| "cancelText"
	| "confirmLoading"
	| "disabled"
	| "hideCancel"
> {
	readonly inactiveWorkspacesToGoDormant: number;
	readonly inactiveWorkspacesToGoDormantInWeek: number;
	readonly dormantWorkspacesToBeDeleted: number;
	readonly dormantWorkspacesToBeDeletedInWeek: number;
	readonly updateDormantWorkspaces: (confirm: boolean) => void;
	readonly updateInactiveWorkspaces: (confirm: boolean) => void;
	readonly dormantWorkspacesChecked: boolean;
	readonly inactiveWorkspacesChecked: boolean;
	readonly dormantValueChanged: boolean;
	readonly deletionValueChanged: boolean;
}

export const ScheduleDialog: FC<ScheduleDialogProps> = ({
	cancelText,
	confirmLoading,
	disabled = false,
	hideCancel = false,
	onClose,
	onConfirm,
	open = false,
	title,
	inactiveWorkspacesToGoDormant,
	inactiveWorkspacesToGoDormantInWeek,
	dormantWorkspacesToBeDeleted,
	dormantWorkspacesToBeDeletedInWeek,
	updateDormantWorkspaces,
	updateInactiveWorkspaces,
	dormantWorkspacesChecked,
	inactiveWorkspacesChecked,
	dormantValueChanged,
	deletionValueChanged,
}) => {
	const showDormancyWarning =
		dormantValueChanged &&
		(inactiveWorkspacesToGoDormant > 0 ||
			inactiveWorkspacesToGoDormantInWeek > 0);
	const showDeletionWarning =
		deletionValueChanged &&
		(dormantWorkspacesToBeDeleted > 0 ||
			dormantWorkspacesToBeDeletedInWeek > 0);

	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (!nextOpen) {
					onClose();
				}
			}}
		>
			<DialogContent
				variant="destructive"
				data-testid="dialog"
				aria-describedby={undefined}
			>
				<DialogHeader>
					<DialogTitle>{title}</DialogTitle>
				</DialogHeader>

				<div className="flex flex-col gap-4 text-sm text-content-secondary font-medium [&_strong]:text-content-primary">
					{showDormancyWarning && (
						<div className="flex flex-col gap-3">
							<h4 className="m-0 text-base font-semibold text-content-primary">
								Dormancy Threshold
							</h4>
							<p className="m-0 leading-relaxed">
								This change will result in{" "}
								<strong>{inactiveWorkspacesToGoDormant}</strong>{" "}
								{inactiveWorkspacesToGoDormant === 1
									? "workspace"
									: "workspaces"}{" "}
								being immediately transitioned to the dormant state and{" "}
								<strong>{inactiveWorkspacesToGoDormantInWeek}</strong>{" "}
								{inactiveWorkspacesToGoDormantInWeek === 1
									? "workspace"
									: "workspaces"}{" "}
								over the next 7 days. To prevent this, do you want to reset the
								inactivity period for all template workspaces?
							</p>
							<label
								htmlFor="prevent-dormancy"
								className="flex items-center gap-2 text-content-primary"
							>
								<Checkbox
									id="prevent-dormancy"
									checked={inactiveWorkspacesChecked}
									onCheckedChange={(checked) => {
										updateInactiveWorkspaces(checked === true);
									}}
								/>
								<span>
									Prevent Dormancy - Reset all workspace inactivity periods
								</span>
							</label>
						</div>
					)}

					{showDeletionWarning && (
						<div className="flex flex-col gap-3">
							<h4 className="m-0 text-base font-semibold text-content-primary">
								Dormancy Auto-Deletion
							</h4>
							<p className="m-0 leading-relaxed">
								This change will result in{" "}
								<strong>{dormantWorkspacesToBeDeleted}</strong>{" "}
								{dormantWorkspacesToBeDeleted === 1
									? "workspace"
									: "workspaces"}{" "}
								being immediately deleted and{" "}
								<strong>{dormantWorkspacesToBeDeletedInWeek}</strong>{" "}
								{dormantWorkspacesToBeDeletedInWeek === 1
									? "workspace"
									: "workspaces"}{" "}
								over the next 7 days. To prevent this, do you want to reset the
								dormancy period for all template workspaces?
							</p>
							<label
								htmlFor="prevent-deletion"
								className="flex items-center gap-2 text-content-primary"
							>
								<Checkbox
									id="prevent-deletion"
									checked={dormantWorkspacesChecked}
									onCheckedChange={(checked) => {
										updateDormantWorkspaces(checked === true);
									}}
								/>
								<span>
									Prevent Deletion - Reset all workspace dormancy periods
								</span>
							</label>
						</div>
					)}
				</div>

				<DialogFooter>
					<DialogActions
						cancelText={cancelText}
						confirmLoading={confirmLoading}
						confirmText="Submit"
						confirmDisabled={disabled}
						confirmVariant="destructive"
						onCancel={!hideCancel ? onClose : undefined}
						onConfirm={onConfirm || onClose}
					/>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
