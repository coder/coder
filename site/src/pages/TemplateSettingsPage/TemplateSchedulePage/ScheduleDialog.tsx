import type { Interpolation, Theme } from "@emotion/react";
import Checkbox from "@mui/material/Checkbox";
import DialogActions from "@mui/material/DialogActions";
import FormControlLabel from "@mui/material/FormControlLabel";
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
		<Dialog
			css={styles.dialogWrapper}
			onClose={onClose}
			open={open}
			data-testid="dialog"
		>
			<div css={styles.dialogContent}>
				<h3 css={styles.dialogTitle}>{title}</h3>

				{showDormancyWarning && (
					<>
						<h4>Dormancy threshold</h4>
						<p css={styles.dialogDescription}>
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
							css={{ marginTop: 16 }}
							control={
								<Checkbox
									size="small"
									onChange={(e) => {
										updateInactiveWorkspaces(e.target.checked);
									}}
								/>
							}
							label="Prevent dormancy - Reset all workspace inactivity periods"
						/>
					</>
				)}

				{showDeletionWarning && (
					<>
						<h4>Dormancy Auto-Deletion</h4>
						<p css={styles.dialogDescription}>
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
							css={{ marginTop: 16 }}
							control={
								<Checkbox
									size="small"
									onChange={(e) => {
										updateDormantWorkspaces(e.target.checked);
									}}
								/>
							}
							label="Prevent deletion - Reset all workspace dormancy periods"
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

const styles = {
	dialogWrapper: (theme) => ({
		"& .MuiPaper-root": {
			background: theme.palette.background.paper,
			border: `1px solid ${theme.palette.divider}`,
		},
		"& .MuiDialogActions-spacing": {
			padding: "0 40px 40px",
		},
	}),
	dialogContent: (theme) => ({
		color: theme.palette.text.secondary,
		padding: 40,
	}),
	dialogTitle: (theme) => ({
		margin: 0,
		marginBottom: 16,
		color: theme.palette.text.primary,
		fontWeight: 400,
		fontSize: 20,
	}),
	dialogDescription: (theme) => ({
		color: theme.palette.text.secondary,
		lineHeight: "160%",
		fontSize: 16,

		"& strong": {
			color: theme.palette.text.primary,
		},

		"& p:not(.MuiFormHelperText-root)": {
			margin: 0,
		},

		"& > p": {
			margin: "8px 0",
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
