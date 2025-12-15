import Checkbox from "@mui/material/Checkbox";
import DialogActions from "@mui/material/DialogActions";
import FormControlLabel from "@mui/material/FormControlLabel";
import type { ConfirmDialogProps } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Dialog, DialogActionButtons } from "components/Dialogs/Dialog";
import type { FC } from "react";
import { cn } from "utils/cn";

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
		<Dialog
			className={classNames.dialogWrapper}
			onClose={onClose}
			open={open}
			data-testid="dialog"
		>
			<div className={classNames.dialogContent}>
				<h3 className={classNames.dialogTitle}>{title}</h3>

				{showDormancyWarning && (
					<>
						<h4>Dormancy Threshold</h4>
						<p className={classNames.dialogDescription}>
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
						<FormControlLabel
							className="mt-4"
							control={
								<Checkbox
									size="small"
									onChange={(e) => {
										updateInactiveWorkspaces(e.target.checked);
									}}
								/>
							}
							label="Prevent Dormancy - Reset all workspace inactivity periods"
						/>
					</>
				)}

				{showDeletionWarning && (
					<>
						<h4>Dormancy Auto-Deletion</h4>
						<p className={classNames.dialogDescription}>
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
						<FormControlLabel
							className="mt-4"
							control={
								<Checkbox
									size="small"
									onChange={(e) => {
										updateDormantWorkspaces(e.target.checked);
									}}
								/>
							}
							label="Prevent Deletion - Reset all workspace dormancy periods"
						/>
					</>
				)}
			</div>

			<DialogActions>
				<DialogActionButtons
					cancelText={cancelText}
					confirmLoading={confirmLoading}
					confirmText="Submit"
					disabled={disabled}
					onCancel={!hideCancel ? onClose : undefined}
					onConfirm={onConfirm || onClose}
					type="delete"
				/>
			</DialogActions>
		</Dialog>
	);
};

const classNames = {
	dialogWrapper: cn(
		"[&_.MuiPaper-root]:bg-surface-secondary [&_.MuiPaper-root]:border",
		"[&_.MuiPaper-root]:border-solid [&_.MuiPaper-root]:border-zinc-700",
		"[&_.MuiDialogActions-spacing]:pt-0 [&_.MuiDialogActions-spacing]:px-10",
		"[&_.MuiDialogActions-spacing]:pb-10",
	),
	dialogContent: "text-content-secondary p-10",
	dialogTitle: "m-0 mb-4 text-content-primary font-base text-xl",
	dialogDescription: cn(
		"text-content-secondary leading-relaxed text-base",
		"[&_strong]:text-content-primary",
		"[&_p:not(.MuiFormHelperText-root)]:m-0",
		"[&_>p]:my-2",
	),
};
