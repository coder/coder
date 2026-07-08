import type { FormikContextType } from "formik";
import { type FC, useId } from "react";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import {
	InputGroup,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { IconPickerField } from "./IconPickerField";
import { MCPServerAuthSection } from "./MCPServerAuthSection";
import { MCPServerBehaviorSection } from "./MCPServerBehaviorSection";
import { CollapsibleSection, Field } from "./MCPServerFormFieldPrimitives";
import {
	type MCPServerFormValues,
	slugify,
	TRANSPORT_OPTIONS,
} from "./mcpServerFormLogic";

interface MCPServerFormFieldsProps {
	form: FormikContextType<MCPServerFormValues>;
	isSaving: boolean;
	isDisabled: boolean;
	canSubmit: boolean;
	isEditing: boolean;
	onCancel: () => void;
	showDetails: boolean;
	setShowDetails: (open: boolean) => void;
	showAuth: boolean;
	setShowAuth: (open: boolean) => void;
	showBehavior: boolean;
	setShowBehavior: (open: boolean) => void;
}

export const MCPServerFormFields: FC<MCPServerFormFieldsProps> = ({
	form,
	isSaving,
	isDisabled,
	canSubmit,
	isEditing,
	onCancel,
	showDetails,
	setShowDetails,
	showAuth,
	setShowAuth,
	showBehavior,
	setShowBehavior,
}) => {
	const formId = useId();

	return (
		<div className="border border-solid p-6 rounded-lg">
			<form
				onSubmit={form.handleSubmit}
				spellCheck={false}
				autoComplete="off"
				className="flex flex-col gap-6"
			>
				<div className="grid items-start gap-4 sm:grid-cols-2">
					<Field label="Slug" htmlFor={`${formId}-slug`} required>
						<Input
							id={`${formId}-slug`}
							className="placeholder:text-content-disabled shadow-none"
							value={form.values.slug}
							onChange={(event) => {
								void form.setFieldValue("slugTouched", true);
								void form.setFieldValue("slug", event.target.value);
							}}
							placeholder="e.g. github, linear"
							disabled={isDisabled}
						/>
					</Field>
					<Field
						label="Display name"
						htmlFor={`${formId}-display-name`}
						required
					>
						<Input
							id={`${formId}-display-name`}
							className="placeholder:text-content-disabled shadow-none"
							value={form.values.displayName}
							onChange={(event) => {
								void form.setFieldValue("displayName", event.target.value);
								if (!form.values.slugTouched) {
									void form.setFieldValue("slug", slugify(event.target.value));
								}
							}}
							disabled={isDisabled}
						/>
					</Field>
					<div className="grid items-start gap-4 sm:col-span-2 sm:grid-cols-[1fr_224px]">
						<Field label="Server URL" htmlFor={`${formId}-url`} required>
							<InputGroup>
								<InputGroupInput
									id={`${formId}-url`}
									className="placeholder:text-content-disabled"
									{...form.getFieldProps("url")}
									placeholder="https://"
									disabled={isDisabled}
								/>
							</InputGroup>
						</Field>
						<Field label="Transport" htmlFor={`${formId}-transport`} required>
							<Select
								value={form.values.transport}
								onValueChange={(value) =>
									void form.setFieldValue("transport", value)
								}
								disabled={isDisabled}
							>
								<SelectTrigger
									id={`${formId}-transport`}
									className="shadow-none"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{TRANSPORT_OPTIONS.map((option) => (
										<SelectItem key={option.value} value={option.value}>
											{option.label}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</Field>
					</div>
				</div>

				<div className="overflow-hidden rounded-lg border border-solid border-border">
					<CollapsibleSection
						title="Details"
						description="Optional description and icon shown to users."
						open={showDetails}
						onOpenChange={setShowDetails}
						contentClassName="grid items-start gap-4 pt-5 pl-6 sm:grid-cols-2"
					>
						<Field label="Description" htmlFor={`${formId}-description`}>
							<Input
								id={`${formId}-description`}
								className="placeholder:text-content-disabled shadow-none"
								{...form.getFieldProps("description")}
								disabled={isDisabled}
							/>
						</Field>
						<Field label="Icon" htmlFor={`${formId}-icon`}>
							<IconPickerField
								id={`${formId}-icon`}
								value={form.values.iconURL}
								placeholder="file location"
								onChange={(value) => void form.setFieldValue("iconURL", value)}
								disabled={isDisabled}
							/>
						</Field>
					</CollapsibleSection>

					<CollapsibleSection
						title="Authentication"
						description="How users authenticate with this MCP server."
						open={showAuth}
						onOpenChange={setShowAuth}
						className="border-0 border-t border-solid border-border"
						contentClassName="space-y-5 pt-5 pl-6"
					>
						<MCPServerAuthSection
							form={form}
							formId={formId}
							disabled={isDisabled}
						/>
					</CollapsibleSection>

					<CollapsibleSection
						title="Behavior"
						description="Availability, model intent, identity headers, and tool governance."
						open={showBehavior}
						onOpenChange={setShowBehavior}
						className="border-0 border-t border-solid border-border"
						contentClassName="space-y-6 pt-5 pl-6"
					>
						<MCPServerBehaviorSection
							form={form}
							formId={formId}
							disabled={isDisabled}
						/>
					</CollapsibleSection>
				</div>

				<div className="flex justify-end gap-4">
					<Button
						variant="outline"
						type="button"
						onClick={onCancel}
						disabled={isDisabled}
					>
						Cancel
					</Button>
					<Button disabled={!canSubmit} type="submit">
						<Spinner loading={isSaving} />
						{isEditing ? "Update server" : "Add server"}
					</Button>
				</div>
			</form>
		</div>
	);
};
