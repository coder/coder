import { useFormik } from "formik";
import { type FC, useState } from "react";
import { SliderPicker, TwitterPicker } from "react-color";
import type { BannerConfig } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogActions,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { AnnouncementBannerView } from "#/modules/dashboard/AnnouncementBanners/AnnouncementBannerView";
import { useTheme } from "#/theme/context";
import { cn } from "#/utils/cn";
import { getFormHelpers } from "#/utils/formUtils";

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
		<Dialog
			open
			onOpenChange={(nextOpen) => {
				if (!nextOpen) {
					onCancel();
				}
			}}
		>
			{/* Banner preview */}
			<div className="fixed top-0 left-0 right-0 z-[60]">
				<AnnouncementBannerView
					message={bannerForm.values.message}
					backgroundColor={bannerForm.values.background_color}
				/>
			</div>

			<DialogContent
				className="max-w-[500px]"
				data-testid="dialog"
				aria-describedby={undefined}
			>
				<DialogHeader>
					<DialogTitle>Announcement banner</DialogTitle>
				</DialogHeader>

				<div className="flex flex-col gap-4">
					<div>
						<h4 className="m-0 mb-2 text-base font-semibold text-content-primary">
							Message
						</h4>
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
						<h4 className="m-0 mb-2 text-base font-semibold text-content-primary">
							Background color
						</h4>
						<div className="flex flex-col gap-4">
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
						</div>
					</div>
				</div>

				<DialogFooter>
					<DialogActions
						cancelText="Cancel"
						confirmLoading={bannerForm.isSubmitting}
						confirmText="Update"
						confirmDisabled={bannerForm.isSubmitting}
						onCancel={onCancel}
						onConfirm={bannerForm.handleSubmit}
					/>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
