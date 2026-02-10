import { Checkbox } from "components/Checkbox/Checkbox";
import type { ConfirmDialogProps } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Dialog, DialogActionButtons } from "components/Dialogs/Dialog";
import type { FC } from "react";

interface ScheduleDialogProps extends ConfirmDialogProps {
	readonly inactiveWorkspacesToGoDormant: number;
	readonly inactiveWorkspacesToGoDormantInWeek: number;
	readonly dormantWorkspacesToBeDeleted: number;
	readonly dormantWorkspacesToBeDeletedInWeek: number;
	readonly updateDormantWorkspaces: (confirm: boolean) => void;
	readonly updateInactiveWorkspaces: (confirm: boolean) => void;
	readonly dormantValueChanged: boolean;
	readonly deletionValueChanged: boolean;
}

export const ScheduleDialog: FC<ScheduleDialogProps> = ({
	cancelText,
	confirmLoading,
	disabled = false,
	hideCancel,
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
	dormantValueChanged,
	deletionValueChanged,
}) => {
	const defaults = {
		confirmText: "Delete",
		hideCancel: false,
	};

	if (typeof hideCancel === "undefined") {
		hideCancel = defaults.hideCancel;
	}

	const showDormancyWarning =
		dormantValueChanged &&
		(inactiveWorkspacesToGoDormant > 0 ||
			inactiveWorkspacesToGoDormantInWeek > 0);
	const showDeletionWarning =
		deletionValueChanged &&
		(dormantWorkspacesToBeDeleted > 0 ||
			dormantWorkspacesToBeDeletedInWeek > 0);

	return (
		<Dialog onClose={onClose} open={open} data-testid="dialog">
			<div className="text-content-secondary p-10">
				<h3 className="m-0 mb-4 text-content-primary font-normal text-xl">
					{title}
				</h3>

				{showDormancyWarning && (
					<>
						<h4>Dormancy Threshold</h4>
						<p className="text-content-secondary leading-[160%] text-base [&_strong]:text-content-primary m-0 my-2">
							This change will result in{" "}
							<strong>{inactiveWorkspacesToGoDormant}</strong>{" "}
							{inactiveWorkspacesToGoDormant === 1 ? "workspace" : "workspaces"}{" "}
							being immediately transitioned to the dormant state and{" "}
							<strong>{inactiveWorkspacesToGoDormantInWeek}</strong>{" "}
							{inactiveWorkspacesToGoDormantInWeek === 1
								? "workspace"
								: "workspaces"}{" "}
							over the next 7 days. To prevent this, do you want to reset the
							inactivity period for all template workspaces?
						</p>
						<div className="flex items-center gap-3 mt-4">
							<Checkbox
								id="prevent-dormancy"
								onCheckedChange={(checked) => {
									updateInactiveWorkspaces(checked === true);
								}}
							/>
							<label
								htmlFor="prevent-dormancy"
								className="text-sm cursor-pointer"
							>
								Prevent Dormancy - Reset all workspace inactivity periods
							</label>
						</div>
					</>
				)}

				{showDeletionWarning && (
					<>
						<h4>Dormancy Auto-Deletion</h4>
						<p className="text-content-secondary leading-[160%] text-base [&_strong]:text-content-primary m-0 my-2">
							This change will result in{" "}
							<strong>{dormantWorkspacesToBeDeleted}</strong>{" "}
							{dormantWorkspacesToBeDeleted === 1 ? "workspace" : "workspaces"}{" "}
							being immediately deleted and{" "}
							<strong>{dormantWorkspacesToBeDeletedInWeek}</strong>{" "}
							{dormantWorkspacesToBeDeletedInWeek === 1
								? "workspace"
								: "workspaces"}{" "}
							over the next 7 days. To prevent this, do you want to reset the
							dormancy period for all template workspaces?
						</p>
						<div className="flex items-center gap-3 mt-4">
							<Checkbox
								id="prevent-deletion"
								onCheckedChange={(checked) => {
									updateDormantWorkspaces(checked === true);
								}}
							/>
							<label
								htmlFor="prevent-deletion"
								className="text-sm cursor-pointer"
							>
								Prevent Deletion - Reset all workspace dormancy periods
							</label>
						</div>
					</>
				)}
			</div>

			<div className="flex justify-end gap-2 px-10 pb-10">
				<DialogActionButtons
					cancelText={cancelText}
					confirmLoading={confirmLoading}
					confirmText="Submit"
					disabled={disabled}
					onCancel={!hideCancel ? onClose : undefined}
					onConfirm={onConfirm || onClose}
					type="delete"
				/>
			</div>
		</Dialog>
	);
};
