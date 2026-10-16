import { cn } from "cn";
import { type FormikErrors, useFormik } from "formik";
import { UploadIcon } from "lucide-react";
import { useId, useRef, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import * as Yup from "yup";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { formatKiB } from "#/utils/fileSize";
import { readAgentAttachmentText } from "../utils/fileAttachmentLimits";
import {
	buildSkillMarkdown,
	getSkillContentSizeBytes,
	isValidSkillDescription,
	isValidSkillName,
	SKILL_MAX_SIZE_BYTES,
	type SkillFormValues,
	tryParseSkillMarkdown,
} from "../utils/skills";

export type SkillErrorDisplay = {
	message: string;
	detail?: string;
};

/**
 * Starts a skill save. Call onSettled when the request finishes so the form
 * accepts another submit, and report failures through submitError.
 */
export type SkillSubmitHandler = (
	values: SkillFormValues,
	content: string,
	onSettled: () => void,
) => void;

type SkillEditorFormState = {
	isBusy: boolean;
	canSubmit: boolean;
	isDirty: boolean;
};

type SkillEditorFormProps = {
	mode: "create" | "edit";
	readOnly?: boolean;
	initialValues: SkillFormValues;
	existingNames: readonly string[];
	submitError?: SkillErrorDisplay;
	isSubmitting: boolean;
	nameHint?: string;
	bodyPlaceholder?: string;
	/** Shows the action and automatic-use switches of a project skill. */
	showProjectSettings?: boolean;
	/** Rendered inside the form, above the scrollable fields. */
	header: (state: SkillEditorFormState) => React.ReactNode;
	/** Rendered inside the form, below the fields, so it can hold Submit. */
	footer: (state: SkillEditorFormState) => React.ReactNode;
	onSubmit: SkillSubmitHandler;
};

type SkillEditorProps = Omit<
	SkillEditorFormProps,
	"header" | "footer" | "nameHint" | "bodyPlaceholder" | "showProjectSettings"
> & {
	open: boolean;
	noun: string;
	editorDescription: string;
	onOpenChange: (open: boolean) => void;
	onCloseAutoFocus?: (event: Event) => void;
};

type ImportStatus = {
	kind: "success" | "error";
	title: string;
	detail?: string;
};

const beginsWithFrontmatterDelimiter = (content: string): boolean =>
	content
		.replace(/^\uFEFF/, "")
		.split(/\r?\n/, 1)[0]
		?.trim() === "---";

/** The skill fields and SKILL.md import, without a dialog around them. */
export const SkillEditorForm: React.FC<SkillEditorFormProps> = ({
	mode,
	readOnly = false,
	initialValues,
	existingNames,
	submitError,
	isSubmitting,
	nameHint = "Use lowercase letters, numbers, and hyphens. Names cannot be changed after creation.",
	bodyPlaceholder = "Describe when and how agents should use this skill.",
	showProjectSettings = false,
	header,
	footer,
	onSubmit,
}) => {
	const isCreate = mode === "create";
	const importId = useId();
	const nameId = useId();
	const nameErrorId = useId();
	const descriptionId = useId();
	const descriptionErrorId = useId();
	const bodyId = useId();
	const bodyErrorId = useId();
	const validationSchema = Yup.object({
		name: Yup.string()
			.trim()
			.required("Name is required.")
			.test(
				"skill-name",
				"Use kebab-case with lowercase letters, numbers, and single hyphens, up to 256 bytes.",
				(value) => Boolean(value && isValidSkillName(value.trim())),
			)
			.test(
				"unique-name",
				"A skill with this name already exists.",
				(value) =>
					!isCreate ||
					!existingNames.includes(
						value?.trim().toLocaleLowerCase("en-US") ?? "",
					),
			),
		description: Yup.string().test(
			"description-size",
			"Description must be 4096 bytes or smaller.",
			(value) => isValidSkillDescription(value ?? ""),
		),
		body: Yup.string().test("body-required", "Body is required.", (value) =>
			Boolean(value?.trim()),
		),
	});

	const validate = (values: SkillFormValues): FormikErrors<SkillFormValues> => {
		if (
			getSkillContentSizeBytes(buildSkillMarkdown(values)) <=
			SKILL_MAX_SIZE_BYTES
		) {
			return {};
		}
		return {
			body: `Skill content must be ${formatKiB(SKILL_MAX_SIZE_BYTES)} or smaller.`,
		};
	};

	const form = useFormik<SkillFormValues>({
		initialValues,
		enableReinitialize: true,
		validationSchema,
		validate,
		// Formik keeps isSubmitting set after a synchronous onSubmit until
		// onSettled clears it, so a fast second click cannot submit again
		// before the caller's isSubmitting turns on.
		onSubmit: (values, { setSubmitting }) => {
			const normalizedValues = {
				...values,
				name: values.name.trim(),
				description: values.description.trim(),
				body: values.body.trim(),
			};
			onSubmit(normalizedValues, buildSkillMarkdown(normalizedValues), () =>
				setSubmitting(false),
			);
		},
	});

	const fileInputRef = useRef<HTMLInputElement>(null);
	const [isReadingFile, setIsReadingFile] = useState(false);
	const isBusy = isSubmitting || isReadingFile || form.isSubmitting;
	const [importContent, setImportContent] = useState("");
	const [importStatus, setImportStatus] = useState<ImportStatus | null>(null);

	const importSkillMarkdown = async (contentToImport: string) => {
		if (!contentToImport.trim()) {
			return;
		}

		const result = tryParseSkillMarkdown(contentToImport);
		if (!result.ok) {
			setImportStatus({
				kind: "error",
				title: "Could not parse SKILL.md",
				detail: result.error,
			});
			return;
		}

		if (isCreate) {
			// Automatic use follows each imported file's frontmatter.
			await form.setValues({
				...form.values,
				...(showProjectSettings && { modelInvocable: true }),
				...result.values,
			});
			await form.setTouched(
				{ name: true, description: true, body: true },
				false,
			);
		} else {
			await form.setValues({
				...form.values,
				description: result.values.description,
				body: result.values.body,
			});
			await form.setTouched(
				{ name: false, description: true, body: true },
				false,
			);
		}

		setImportContent("");
		setImportStatus({
			kind: "success",
			title: "Imported SKILL.md",
			detail: isCreate
				? showProjectSettings
					? "Updated name, description, body, and automatic use."
					: "Updated name, description, and body fields."
				: "Updated description and body fields. Kept the existing name.",
		});
	};

	const handleFileChange = async (
		event: React.ChangeEvent<HTMLInputElement>,
	) => {
		const file = event.currentTarget.files?.[0];
		event.currentTarget.value = "";
		if (!file) {
			return;
		}

		setImportStatus(null);
		if (file.size > SKILL_MAX_SIZE_BYTES) {
			setImportStatus({
				kind: "error",
				title: "File is too large",
				detail: `Choose a file that is ${formatKiB(SKILL_MAX_SIZE_BYTES)} or smaller.`,
			});
			return;
		}

		setIsReadingFile(true);
		try {
			const content = await readAgentAttachmentText(file);
			if (!content.trim()) {
				setImportStatus({
					kind: "error",
					title: "File is empty",
					detail: "Choose a SKILL.md file with frontmatter and a body.",
				});
			} else {
				setImportContent(content);
				await importSkillMarkdown(content);
			}
		} catch {
			setImportStatus({
				kind: "error",
				title: "Could not read file",
				detail: "Try selecting the file again.",
			});
		}
		setIsReadingFile(false);
	};

	const handleImportContentChange = (
		event: React.ChangeEvent<HTMLTextAreaElement>,
	) => {
		setImportContent(event.target.value);
		setImportStatus(null);
	};

	const handleImportContentPaste = (
		event: React.ClipboardEvent<HTMLTextAreaElement>,
	) => {
		const pastedContent = event.clipboardData.getData("text");
		if (!beginsWithFrontmatterDelimiter(pastedContent)) {
			return;
		}

		event.preventDefault();
		setImportContent(pastedContent);
		setImportStatus(null);
		void importSkillMarkdown(pastedContent);
	};

	const content = buildSkillMarkdown(form.values);
	const sizeBytes = getSkillContentSizeBytes(content);
	const nameError = form.touched.name ? form.errors.name : undefined;
	const descriptionError = form.touched.description
		? form.errors.description
		: undefined;
	const bodyError = form.touched.body ? form.errors.body : undefined;
	const isTooLarge = sizeBytes > SKILL_MAX_SIZE_BYTES;
	const isNearLimit = sizeBytes > SKILL_MAX_SIZE_BYTES * 0.9;
	const formState: SkillEditorFormState = {
		isBusy,
		canSubmit: !isBusy && form.isValid && form.dirty,
		isDirty: form.dirty,
	};

	return (
		<form className="flex min-h-0 flex-1 flex-col" onSubmit={form.handleSubmit}>
			{header(formState)}

			<div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-6 py-4">
				{submitError && (
					<Alert severity="error">
						<AlertTitle>{submitError.message}</AlertTitle>
						{submitError.detail && (
							<AlertDescription>{submitError.detail}</AlertDescription>
						)}
					</Alert>
				)}

				{!readOnly && (
					<div className="flex flex-col gap-3 rounded-md border border-border p-4">
						<div className="flex flex-col gap-1">
							<Label htmlFor={importId}>Import from SKILL.md</Label>
							<p className="m-0 text-xs text-content-secondary">
								Upload or paste a full SKILL.md file with frontmatter to
								auto-fill the fields below.
							</p>
						</div>
						<TextareaAutosize
							id={importId}
							value={importContent}
							onChange={handleImportContentChange}
							onPaste={handleImportContentPaste}
							placeholder={
								"---\nname: my-skill\ndescription: ...\n---\n\nBody..."
							}
							disabled={isBusy}
							minRows={4}
							maxRows={10}
							className="w-full resize-y rounded-md border border-border bg-transparent px-3 py-2 font-mono text-sm leading-relaxed text-content-primary placeholder:text-content-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link disabled:cursor-not-allowed disabled:opacity-50"
						/>
						{importStatus && (
							<Alert severity={importStatus.kind}>
								<AlertTitle>{importStatus.title}</AlertTitle>
								{importStatus.detail && (
									<AlertDescription>{importStatus.detail}</AlertDescription>
								)}
							</Alert>
						)}
						<div className="flex flex-wrap justify-end gap-2">
							<input
								ref={fileInputRef}
								type="file"
								accept=".md,.markdown,.txt,text/markdown,text/plain"
								aria-label="Upload SKILL.md"
								className="hidden"
								disabled={isBusy}
								onChange={handleFileChange}
							/>
							<Button
								variant="outline"
								size="sm"
								className="mr-auto"
								disabled={isBusy}
								onClick={() => fileInputRef.current?.click()}
							>
								<Spinner loading={isReadingFile}>
									<UploadIcon />
								</Spinner>
								Upload file
							</Button>
							{importContent && (
								<Button
									variant="outline"
									size="sm"
									disabled={isBusy}
									onClick={() => {
										setImportContent("");
										setImportStatus(null);
									}}
								>
									Clear
								</Button>
							)}
							<Button
								size="sm"
								disabled={isBusy || !importContent.trim()}
								onClick={() => {
									void importSkillMarkdown(importContent);
								}}
							>
								Import
							</Button>
						</div>
					</div>
				)}
				<div className="flex flex-col gap-2">
					<Label htmlFor={nameId}>Name</Label>
					<Input
						id={nameId}
						name="name"
						value={form.values.name}
						onChange={form.handleChange}
						onBlur={form.handleBlur}
						placeholder="review-database-query"
						// Names cannot change after creation. readOnly, unlike
						// disabled, keeps the name focusable and selectable.
						readOnly={readOnly || !isCreate}
						disabled={isBusy}
						aria-invalid={Boolean(nameError)}
						aria-describedby={nameError ? nameErrorId : undefined}
						className={cn((readOnly || !isCreate) && "bg-surface-secondary")}
					/>
					{nameError ? (
						<p
							id={nameErrorId}
							className="m-0 text-xs text-content-destructive"
						>
							{nameError}
						</p>
					) : (
						isCreate && (
							<p className="m-0 text-xs text-content-secondary">{nameHint}</p>
						)
					)}
				</div>

				<div className="flex flex-col gap-2">
					<Label htmlFor={descriptionId}>Description</Label>
					<Input
						id={descriptionId}
						name="description"
						value={form.values.description}
						onChange={form.handleChange}
						onBlur={form.handleBlur}
						placeholder="When to use this skill"
						readOnly={readOnly}
						disabled={isBusy}
						aria-invalid={Boolean(descriptionError)}
						aria-describedby={descriptionError ? descriptionErrorId : undefined}
						className={cn(readOnly && "bg-surface-secondary")}
					/>
					{descriptionError && (
						<p
							id={descriptionErrorId}
							className="m-0 text-xs text-content-destructive"
						>
							{descriptionError}
						</p>
					)}
				</div>

				{showProjectSettings && (
					<>
						<SkillSettingSwitch
							label="Show as action"
							description="Adds a button above the composer in this project's chats that inserts the skill."
							checked={form.values.action ?? false}
							disabled={readOnly || isBusy}
							onCheckedChange={(checked) =>
								void form.setFieldValue("action", checked)
							}
						/>
						<SkillSettingSwitch
							label="Agents can use it automatically"
							description="When off, agents use the skill only when someone invokes it."
							checked={form.values.modelInvocable ?? true}
							disabled={readOnly || isBusy}
							onCheckedChange={(checked) =>
								void form.setFieldValue("modelInvocable", checked)
							}
						/>
					</>
				)}

				<div className="flex flex-col gap-2">
					<Label htmlFor={bodyId}>Body</Label>
					<TextareaAutosize
						id={bodyId}
						name="body"
						value={form.values.body}
						onChange={form.handleChange}
						onBlur={form.handleBlur}
						placeholder={bodyPlaceholder}
						readOnly={readOnly}
						disabled={isBusy}
						minRows={8}
						aria-invalid={Boolean(bodyError)}
						aria-describedby={bodyError ? bodyErrorId : undefined}
						className={cn(
							"w-full resize-y rounded-md border border-border bg-transparent px-3 py-2 font-mono text-sm leading-relaxed text-content-primary placeholder:text-content-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link disabled:cursor-not-allowed disabled:opacity-50",
							bodyError && "border-border-destructive",
							readOnly && "bg-surface-secondary",
						)}
					/>
					{bodyError && (
						<p
							id={bodyErrorId}
							className="m-0 text-xs text-content-destructive"
						>
							{bodyError}
						</p>
					)}
					<p
						className={cn(
							"m-0 text-xs text-content-secondary",
							isNearLimit && "text-content-warning",
							isTooLarge && "text-content-destructive",
						)}
					>
						{`${formatKiB(sizeBytes)} of ${formatKiB(SKILL_MAX_SIZE_BYTES)} used.`}
					</p>
				</div>
			</div>

			{footer(formState)}
		</form>
	);
};

type SkillSettingSwitchProps = {
	label: string;
	description: string;
	checked: boolean;
	disabled: boolean;
	onCheckedChange: (checked: boolean) => void;
};

const SkillSettingSwitch: React.FC<SkillSettingSwitchProps> = ({
	label,
	description,
	checked,
	disabled,
	onCheckedChange,
}) => {
	const switchId = useId();
	const descriptionId = useId();
	return (
		<div className="flex items-start justify-between gap-4">
			<div className="flex flex-col gap-1">
				<Label htmlFor={switchId}>{label}</Label>
				<p id={descriptionId} className="m-0 text-xs text-content-secondary">
					{description}
				</p>
			</div>
			<Switch
				id={switchId}
				aria-describedby={descriptionId}
				checked={checked}
				disabled={disabled}
				onCheckedChange={onCheckedChange}
			/>
		</div>
	);
};

/** Creates, edits, or views a skill in a dialog. */
export const SkillEditor: React.FC<SkillEditorProps> = ({
	open,
	noun,
	editorDescription,
	onOpenChange,
	onCloseAutoFocus,
	...formProps
}) => {
	const closeButtonRef = useRef<HTMLButtonElement>(null);
	const { mode, readOnly, isSubmitting } = formProps;
	const lowerNoun = noun.toLocaleLowerCase("en-US");
	const title = readOnly
		? `View ${lowerNoun}`
		: `${mode === "create" ? "Create" : "Edit"} ${lowerNoun}`;

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				className="flex max-h-[90vh] max-w-2xl flex-col gap-0 overflow-hidden p-0"
				onCloseAutoFocus={onCloseAutoFocus}
				onOpenAutoFocus={(event) => {
					if (readOnly) {
						event.preventDefault();
						closeButtonRef.current?.focus();
					}
				}}
			>
				<SkillEditorForm
					{...formProps}
					header={() => (
						<DialogHeader className="px-6 pt-6">
							<DialogTitle>{title}</DialogTitle>
							<DialogDescription>{editorDescription}</DialogDescription>
						</DialogHeader>
					)}
					footer={({ isBusy, canSubmit }) => (
						<DialogFooter className="border-t border-border px-6 py-4">
							<Button
								ref={closeButtonRef}
								variant="outline"
								disabled={isBusy}
								onClick={() => onOpenChange(false)}
							>
								{readOnly ? "Close" : "Cancel"}
							</Button>
							{!readOnly && (
								<Button type="submit" disabled={!canSubmit}>
									{isSubmitting && <Spinner className="size-4" loading />}
									{mode === "create" ? "Create skill" : "Save skill"}
								</Button>
							)}
						</DialogFooter>
					)}
				/>
			</DialogContent>
		</Dialog>
	);
};
