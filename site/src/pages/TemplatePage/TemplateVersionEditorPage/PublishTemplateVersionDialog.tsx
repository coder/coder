import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import { EnterpriseBadge } from "#/components/Badges/Badges";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { FormFields } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverLink,
	HelpPopoverLinksGroup,
	HelpPopoverText,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import { Label } from "#/components/Label/Label";
import { Textarea } from "#/components/Textarea/Textarea";
import type { PublishVersionData } from "#/pages/TemplatePage/TemplateVersionEditorPage/types";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";
import { getFormHelpers } from "#/utils/formUtils";

type PublishTemplateVersionDialogProps = {
	open: boolean;
	defaultName: string;
	isPublishing: boolean;
	publishingError?: unknown;
	onClose: () => void;
	onConfirm: (data: PublishVersionData) => void;
};

export const PublishTemplateVersionDialog: FC<
	PublishTemplateVersionDialogProps
> = ({
	open,
	onConfirm,
	isPublishing,
	onClose,
	defaultName,
	publishingError,
}) => {
	const form = useFormik({
		initialValues: {
			name: defaultName,
			message: "",
			isActiveVersion: true,
		},
		validationSchema: Yup.object({
			name: Yup.string().required(),
			message: Yup.string(),
			isActiveVersion: Yup.boolean(),
		}),
		onSubmit: onConfirm,
	});
	const getFieldHelpers = getFormHelpers(form, publishingError);
	const messageField = getFieldHelpers("message");
	const messageErrorId = `${messageField.id}-error`;
	const handleClose = () => {
		form.resetForm();
		onClose();
	};

	return (
		<ConfirmDialog
			open={open}
			confirmLoading={isPublishing}
			onClose={handleClose}
			onConfirm={async () => {
				await form.submitForm();
			}}
			hideCancel={false}
			type="success"
			cancelText="Cancel"
			confirmText="Publish"
			title="Publish new version"
			description={
				<form id="publish-version" onSubmit={form.handleSubmit}>
					<div className="flex flex-col gap-4">
						<p>You are about to publish a new version of this template.</p>
						<FormFields>
							<FormField
								field={getFieldHelpers("name")}
								label="Version name"
								autoFocus
								disabled={isPublishing}
							/>

							<div className="flex flex-col gap-2">
								<Label htmlFor={messageField.id}>Message</Label>
								<Textarea
									id={messageField.id}
									name={messageField.name}
									value={messageField.value ?? ""}
									onChange={messageField.onChange}
									onBlur={messageField.onBlur}
									placeholder="Write a short message about the changes you made..."
									disabled={isPublishing}
									rows={5}
									aria-invalid={messageField.error}
									aria-describedby={
										messageField.error ? messageErrorId : undefined
									}
									className={cn(
										messageField.error && "border-border-destructive",
									)}
								/>
								{messageField.error && (
									<span
										id={messageErrorId}
										className="text-xs text-content-destructive"
									>
										{messageField.helperText}
									</span>
								)}
							</div>

							<div className="flex flex-row items-center gap-4">
								<div className="flex items-center gap-2">
									<Checkbox
										id="isActiveVersion"
										checked={form.values.isActiveVersion}
										onCheckedChange={(checked) => {
											void form.setFieldValue(
												"isActiveVersion",
												Boolean(checked),
											);
										}}
										name="isActiveVersion"
									/>
									<Label htmlFor="isActiveVersion" className="cursor-pointer">
										Promote to active version
									</Label>
								</div>

								<HelpPopover>
									<HelpPopoverIconTrigger />
									<HelpPopoverContent>
										<HelpPopoverTitle>Active versions</HelpPopoverTitle>
										<HelpPopoverText>
											Templates can enforce that the active version be used for
											all workspaces <EnterpriseBadge />
										</HelpPopoverText>
										<HelpPopoverLinksGroup>
											<HelpPopoverLink
												href={docs(
													"/admin/templates/managing-templates#template-update-policies",
												)}
											>
												Review the documentation
											</HelpPopoverLink>
										</HelpPopoverLinksGroup>
									</HelpPopoverContent>
								</HelpPopover>
							</div>
						</FormFields>
					</div>
				</form>
			}
		/>
	);
};
