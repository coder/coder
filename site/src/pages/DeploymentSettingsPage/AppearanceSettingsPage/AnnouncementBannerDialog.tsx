import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import DialogActions from "@mui/material/DialogActions";
import TextField from "@mui/material/TextField";
import type { BannerConfig } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Dialog, DialogActionButtons } from "components/Dialogs/Dialog";
import { Stack } from "components/Stack/Stack";
import { useFormik } from "formik";
import { AnnouncementBannerView } from "modules/dashboard/AnnouncementBanners/AnnouncementBannerView";
import { type FC, useState } from "react";
import { SliderPicker, TwitterPicker } from "react-color";
import { getFormHelpers } from "utils/formUtils";

interface AnnouncementBannerDialogProps {
	banner: BannerConfig;
	onCancel: () => void;
	onUpdate: (banner: Partial<BannerConfig>) => Promise<void>;
}

export const AnnouncementBannerDialog: FC<AnnouncementBannerDialogProps> = ({
	banner,
	onCancel,
	onUpdate,
}) => {
	const theme = useTheme();

	const bannerForm = useFormik<{
		message: string;
		background_color: string;
	}>({
		initialValues: {
			message: banner.message ?? "",
			background_color: banner.background_color ?? "#ABB8C3",
		},
		onSubmit: (banner) => onUpdate(banner),
	});
	const bannerFieldHelpers = getFormHelpers(bannerForm);

	const [showHuePicker, setShowHuePicker] = useState(false);

	return (
		<Dialog css={styles.dialogWrapper} open onClose={onCancel}>
			{/* Banner preview */}
			<div css={{ position: "fixed", top: 0, left: 0, right: 0 }}>
				<AnnouncementBannerView
					message={bannerForm.values.message}
					backgroundColor={bannerForm.values.background_color}
				/>
			</div>

			<div css={styles.dialogContent}>
				<h3 css={styles.dialogTitle}>Announcement banner</h3>
				<Stack>
					<div>
						<h4 css={styles.settingName}>Message</h4>
						<TextField
							{...bannerFieldHelpers("message", {
								helperText: "Markdown bold, italics, and links are supported.",
							})}
							fullWidth
							multiline
							inputProps={{
								"aria-label": "Message",
								placeholder: "Enter a message for the banner",
							}}
						/>
					</div>
					<div>
						<h4 css={styles.settingName}>Background color</h4>
						<Stack spacing={2}>
							{showHuePicker ? (
								<SliderPicker
									color={bannerForm.values.background_color}
									onChange={async (color) => {
										await bannerForm.setFieldValue(
											"background_color",
											color.hex,
										);
									}}
								/>
							) : (
								<TwitterPicker
									color={bannerForm.values.background_color}
									onChange={async (color) => {
										await bannerForm.setFieldValue(
											"background_color",
											color.hex,
										);
									}}
									triangle="hide"
									colors={[
										"#8b5cf6",
										"#d94a5d",
										"#f78da7",
										"#d65d0f",
										"#ff6900",
										"#fcb900",
										"#0693e3",

										"#8ed1fc",
										"#4cd473",
										"#abb8c3",
									]}
									styles={{
										default: {
											input: {
												color: "white",
												backgroundColor: theme.palette.background.default,
											},
											body: {
												backgroundColor: "transparent",
												color: "white",
												padding: 0,
											},
											card: {
												backgroundColor: "transparent",
											},
										},
									}}
								/>
							)}
							<div>
								<Button
									variant="outline"
									onClick={() => setShowHuePicker((it) => !it)}
								>
									Show {showHuePicker ? "palette" : "slider"}
								</Button>
							</div>
						</Stack>
					</div>
				</Stack>
			</div>

			<DialogActions>
				<DialogActionButtons
					cancelText="Cancel"
					confirmLoading={bannerForm.isSubmitting}
					confirmText="Update"
					disabled={bannerForm.isSubmitting}
					onCancel={onCancel}
					onConfirm={bannerForm.handleSubmit}
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
			width: "100%",
			maxWidth: 500,
		},
		"& .MuiDialogActions-spacing": {
			padding: "0 40px 40px",
		},
	}),
	dialogContent: (theme) => ({
		color: theme.palette.text.secondary,
		padding: "40px 40px 20px",
	}),
	dialogTitle: (theme) => ({
		margin: 0,
		marginBottom: 16,
		color: theme.palette.text.primary,
		fontWeight: 400,
		fontSize: 20,
	}),
	settingName: (theme) => ({
		marginTop: 0,
		marginBottom: 8,
		color: theme.palette.text.primary,
		fontSize: 16,
		lineHeight: "150%",
		fontWeight: 600,
	}),
} satisfies Record<string, Interpolation<Theme>>;
