import { cn } from "cn";
import { type FormikContextType, type FormikErrors, useFormik } from "formik";
import { UploadIcon } from "lucide-react";
import { createContext, use, useId, useRef, useState } from "react";
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
 * accepts another submit, and show failures with SkillEditorSubmitError.
 */
export type SkillSubmitHandler = (
	values: SkillFormValues,
	content: string,
	onSettled: () => void,
) => void;

type SkillEditorMode = "create" | "edit";

type SkillEditorContextValue = {
	form: FormikContextType<SkillFormValues>;
	mode: SkillEditorMode;
	readOnly: boolean;
	isSubmitting: boolean;
	isBusy: boolean;
	isReadingFile: boolean;
	setIsReadingFile: (isReadingFile: boolean) => void;
};

const SkillEditorContext = createContext<SkillEditorContextValue | null>(null);

const useSkillEditorContext = (): SkillEditorContextValue => {
	const context = use(SkillEditorContext);
	if (!context) {
		throw new Error(
			"Skill editor parts must be rendered inside SkillEditorForm.",
		);
	}
	return context;
};

/** Reads the state of the enclosing SkillEditorForm for a custom header or footer. */
export const useSkillEditorFormState = () => {
	const { form, mode, readOnly, isBusy } = useSkillEditorContext();
	return {
		mode,
		readOnly,
		isBusy,
		canSubmit: !isBusy && form.isValid && form.dirty,
		isDirty: form.dirty,
	};
};

type SkillEditorFormProps = {
	mode: SkillEditorMode;
	readOnly?: boolean;
	initialValues: SkillFormValues;
	existingNames: readonly string[];
	isSubmitting: boolean;
	onSubmit: SkillSubmitHandler;
	children: React.ReactNode;
};

/**
 * Owns the skill form state and validation, and provides them to the
 * SkillEditor* parts rendered inside it.
 */
export const SkillEditorForm: React.FC<SkillEditorFormProps> = ({
	mode,
	readOnly = false,
	initialValues,
	existingNames,
	isSubmitting,
	onSubmit,
	children,
}) => {
	const isCreate = mode === "create";
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

	const [isReadingFile, setIsReadingFile] = useState(false);
	const isBusy = isSubmitting || isReadingFile || form.isSubmitting;

	return (
		<SkillEditorContext
			value={{
				form,
				mode,
				readOnly,
				isSubmitting,
				isBusy,
				isReadingFile,
				setIsReadingFile,
			}}
		>
			<form
				className="flex min-h-0 flex-1 flex-col"
				onSubmit={form.handleSubmit}
			>
				{children}
			</form>
		</SkillEditorContext>
	);
};

type SkillEditorFieldsProps = {
	children: React.ReactNode;
};

/** The scrollable column that holds the skill fields. */
export const SkillEditorFields: React.FC<SkillEditorFieldsProps> = ({
	children,
}) => (
	<div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-6 py-4">
		{children}
	</div>
);

type SkillEditorSubmitErrorProps = {
	error: SkillErrorDisplay | undefined;
};

/** Shows why the last save failed, if it did. */
export const SkillEditorSubmitError: React.FC<SkillEditorSubmitErrorProps> = ({
	error,
}) =>
	error ? (
		<Alert severity="error">
			<AlertTitle>{error.message}</AlertTitle>
			{error.detail && <AlertDescription>{error.detail}</AlertDescription>}
		</Alert>
	) : null;

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

type SkillEditorImportProps = {
	/**
	 * Turns automatic use back on before a create-mode import, so a file
	 * without the setting does not inherit an earlier import's value.
	 */
	resetsModelInvocable?: boolean;
};

/** Fills the fields from an uploaded or pasted SKILL.md file. */
export const SkillEditorImport: React.FC<SkillEditorImportProps> = ({
	resetsModelInvocable = false,
}) => {
	const { form, mode, isBusy, isReadingFile, setIsReadingFile } =
		useSkillEditorContext();
	const isCreate = mode === "create";
	const importId = useId();
	const fileInputRef = useRef<HTMLInputElement>(null);
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
			await form.setValues({
				...form.values,
				...(resetsModelInvocable && { modelInvocable: true }),
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
				? resetsModelInvocable
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

	return (
		<div className="flex flex-col gap-3 rounded-md border border-border p-4">
			<div className="flex flex-col gap-1">
				<Label htmlFor={importId}>Import from SKILL.md</Label>
				<p className="m-0 text-xs text-content-secondary">
					Upload or paste a full SKILL.md file with frontmatter to auto-fill the
					fields below.
				</p>
			</div>
			<TextareaAutosize
				id={importId}
				value={importContent}
				onChange={handleImportContentChange}
				onPaste={handleImportContentPaste}
				placeholder={"---\nname: my-skill\ndescription: ...\n---\n\nBody..."}
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
	);
};

type SkillEditorNameFieldProps = {
	/** Shown below a new skill's name until it has an error. */
	hint?: string;
};

/** The skill name, which can be set only when creating a skill. */
export const SkillEditorNameField: React.FC<SkillEditorNameFieldProps> = ({
	hint = "Use lowercase letters, numbers, and hyphens. Names cannot be changed after creation.",
}) => {
	const { form, mode, readOnly, isBusy } = useSkillEditorContext();
	const nameId = useId();
	const nameErrorId = useId();
	const isCreate = mode === "create";
	const isNameReadOnly = readOnly || !isCreate;
	const nameError = form.touched.name ? form.errors.name : undefined;

	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={nameId}>Name</Label>
			<Input
				id={nameId}
				name="name"
				value={form.values.name}
				onChange={form.handleChange}
				onBlur={form.handleBlur}
				placeholder="review-database-query"
				// readOnly, unlike disabled, keeps the name focusable and
				// selectable.
				readOnly={isNameReadOnly}
				disabled={isBusy}
				aria-invalid={Boolean(nameError)}
				aria-describedby={nameError ? nameErrorId : undefined}
				className={cn(isNameReadOnly && "bg-surface-secondary")}
			/>
			{nameError ? (
				<p id={nameErrorId} className="m-0 text-xs text-content-destructive">
					{nameError}
				</p>
			) : (
				isCreate && <p className="m-0 text-xs text-content-secondary">{hint}</p>
			)}
		</div>
	);
};

/** The skill description, which tells agents when to use the skill. */
export const SkillEditorDescriptionField: React.FC = () => {
	const { form, readOnly, isBusy } = useSkillEditorContext();
	const descriptionId = useId();
	const descriptionErrorId = useId();
	const descriptionError = form.touched.description
		? form.errors.description
		: undefined;

	return (
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
	);
};

type SkillEditorBodyFieldProps = {
	placeholder?: string;
};

/** The skill body, with a meter for the size of the whole SKILL.md. */
export const SkillEditorBodyField: React.FC<SkillEditorBodyFieldProps> = ({
	placeholder = "Describe when and how agents should use this skill.",
}) => {
	const { form, readOnly, isBusy } = useSkillEditorContext();
	const bodyId = useId();
	const bodyErrorId = useId();
	const bodyError = form.touched.body ? form.errors.body : undefined;
	const sizeBytes = getSkillContentSizeBytes(buildSkillMarkdown(form.values));
	const isTooLarge = sizeBytes > SKILL_MAX_SIZE_BYTES;
	const isNearLimit = sizeBytes > SKILL_MAX_SIZE_BYTES * 0.9;

	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={bodyId}>Body</Label>
			<TextareaAutosize
				id={bodyId}
				name="body"
				value={form.values.body}
				onChange={form.handleChange}
				onBlur={form.handleBlur}
				placeholder={placeholder}
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
				<p id={bodyErrorId} className="m-0 text-xs text-content-destructive">
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
	);
};

/** The action and automatic-use switches of a project skill. */
export const SkillEditorProjectSettings: React.FC = () => {
	const { form, readOnly, isBusy } = useSkillEditorContext();
	return (
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

type SkillEditorSubmitButtonProps = {
	children: React.ReactNode;
};

/** Submits the form, and shows a spinner while the save is in flight. */
export const SkillEditorSubmitButton: React.FC<
	SkillEditorSubmitButtonProps
> = ({ children }) => {
	const { isSubmitting } = useSkillEditorContext();
	const { canSubmit } = useSkillEditorFormState();
	return (
		<Button type="submit" disabled={!canSubmit}>
			<Spinner loading={isSubmitting} />
			{children}
		</Button>
	);
};

type SkillEditorDialogProps = {
	open: boolean;
	noun: string;
	editorDescription: string;
	mode: SkillEditorMode;
	readOnly?: boolean;
	initialValues: SkillFormValues;
	existingNames: readonly string[];
	submitError?: SkillErrorDisplay;
	isSubmitting: boolean;
	onOpenChange: (open: boolean) => void;
	onCloseAutoFocus?: (event: Event) => void;
	onSubmit: SkillSubmitHandler;
};

/** Creates, edits, or views a skill in a dialog. */
export const SkillEditorDialog: React.FC<SkillEditorDialogProps> = ({
	open,
	noun,
	editorDescription,
	mode,
	readOnly = false,
	initialValues,
	existingNames,
	submitError,
	isSubmitting,
	onOpenChange,
	onCloseAutoFocus,
	onSubmit,
}) => {
	const closeButtonRef = useRef<HTMLButtonElement>(null);
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
					mode={mode}
					readOnly={readOnly}
					initialValues={initialValues}
					existingNames={existingNames}
					isSubmitting={isSubmitting}
					onSubmit={onSubmit}
				>
					<DialogHeader className="px-6 pt-6">
						<DialogTitle>{title}</DialogTitle>
						<DialogDescription>{editorDescription}</DialogDescription>
					</DialogHeader>
					<SkillEditorFields>
						<SkillEditorSubmitError error={submitError} />
						{!readOnly && <SkillEditorImport />}
						<SkillEditorNameField />
						<SkillEditorDescriptionField />
						<SkillEditorBodyField />
					</SkillEditorFields>
					<SkillEditorDialogFooter
						closeButtonRef={closeButtonRef}
						onClose={() => onOpenChange(false)}
					/>
				</SkillEditorForm>
			</DialogContent>
		</Dialog>
	);
};

type SkillEditorDialogFooterProps = {
	closeButtonRef: React.RefObject<HTMLButtonElement | null>;
	onClose: () => void;
};

const SkillEditorDialogFooter: React.FC<SkillEditorDialogFooterProps> = ({
	closeButtonRef,
	onClose,
}) => {
	const { mode, readOnly, isBusy } = useSkillEditorFormState();
	return (
		<DialogFooter className="border-t border-border px-6 py-4">
			<Button
				ref={closeButtonRef}
				variant="outline"
				disabled={isBusy}
				onClick={onClose}
			>
				{readOnly ? "Close" : "Cancel"}
			</Button>
			{!readOnly && (
				<SkillEditorSubmitButton>
					{mode === "create" ? "Create skill" : "Save skill"}
				</SkillEditorSubmitButton>
			)}
		</DialogFooter>
	);
};
