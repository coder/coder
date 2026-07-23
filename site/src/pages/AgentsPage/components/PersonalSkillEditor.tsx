import { type FormikErrors, useFormik } from "formik";
import {
	type ChangeEvent,
	type ClipboardEvent,
	type FC,
	useId,
	useState,
} from "react";
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
import { cn } from "#/utils/cn";
import { formatKiB } from "#/utils/fileSize";
import {
	buildPersonalSkillMarkdown,
	getPersonalSkillContentSizeBytes,
	isValidPersonalSkillDescription,
	isValidPersonalSkillName,
	PERSONAL_SKILL_MAX_SIZE_BYTES,
	type PersonalSkillFormValues,
	tryParsePersonalSkillMarkdown,
} from "../utils/personalSkills";

export type PersonalSkillErrorDisplay = {
	message: string;
	detail?: string;
};

interface PersonalSkillEditorProps {
	open: boolean;
	mode: "create" | "edit";
	initialValues: PersonalSkillFormValues;
	existingNames: readonly string[];
	submitError?: PersonalSkillErrorDisplay;
	isSubmitting: boolean;
	onOpenChange: (open: boolean) => void;
	onSubmit: (values: PersonalSkillFormValues, content: string) => void;
}

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

export const PersonalSkillEditor: FC<PersonalSkillEditorProps> = ({
	open,
	mode,
	initialValues,
	existingNames,
	submitError,
	isSubmitting,
	onOpenChange,
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
				(value) => Boolean(value && isValidPersonalSkillName(value.trim())),
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
			(value) => isValidPersonalSkillDescription(value ?? ""),
		),
		body: Yup.string().test("body-required", "Body is required.", (value) =>
			Boolean(value?.trim()),
		),
	});

	const validate = (
		values: PersonalSkillFormValues,
	): FormikErrors<PersonalSkillFormValues> => {
		if (
			getPersonalSkillContentSizeBytes(buildPersonalSkillMarkdown(values)) <=
			PERSONAL_SKILL_MAX_SIZE_BYTES
		) {
			return {};
		}
		return {
			body: `Skill content must be ${formatKiB(PERSONAL_SKILL_MAX_SIZE_BYTES)} or smaller.`,
		};
	};

	const form = useFormik<PersonalSkillFormValues>({
		initialValues,
		enableReinitialize: true,
		validationSchema,
		validate,
		onSubmit: (values) => {
			const normalizedValues = {
				name: values.name.trim(),
				description: values.description.trim(),
				body: values.body.trim(),
			};
			onSubmit(normalizedValues, buildPersonalSkillMarkdown(normalizedValues));
		},
	});

	const [importContent, setImportContent] = useState("");
	const [importStatus, setImportStatus] = useState<ImportStatus | null>(null);

	const importSkillMarkdown = async (contentToImport: string) => {
		if (!contentToImport.trim()) {
			return;
		}

		const result = tryParsePersonalSkillMarkdown(contentToImport);
		if (!result.ok) {
			setImportStatus({
				kind: "error",
				title: "Could not parse SKILL.md",
				detail: result.error,
			});
			return;
		}

		if (isCreate) {
			await form.setValues(result.values);
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
				? "Updated name, description, and body fields."
				: "Updated description and body fields. Kept the existing name.",
		});
	};

	const handleImportContentChange = (
		event: ChangeEvent<HTMLTextAreaElement>,
	) => {
		setImportContent(event.target.value);
		setImportStatus(null);
	};

	const handleImportContentPaste = (
		event: ClipboardEvent<HTMLTextAreaElement>,
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

	const content = buildPersonalSkillMarkdown(form.values);
	const sizeBytes = getPersonalSkillContentSizeBytes(content);
	const nameError = form.touched.name ? form.errors.name : undefined;
	const descriptionError = form.touched.description
		? form.errors.description
		: undefined;
	const bodyError = form.touched.body ? form.errors.body : undefined;
	const isTooLarge = sizeBytes > PERSONAL_SKILL_MAX_SIZE_BYTES;
	const isNearLimit = sizeBytes > PERSONAL_SKILL_MAX_SIZE_BYTES * 0.9;
	const title = isCreate ? "Create personal skill" : "Edit personal skill";
	const submitLabel = isCreate ? "Create skill" : "Save skill";

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="flex max-h-[90vh] max-w-2xl flex-col gap-0 overflow-hidden p-0">
				<form
					className="flex min-h-0 flex-1 flex-col"
					onSubmit={form.handleSubmit}
				>
					<DialogHeader className="px-6 pt-6">
						<DialogTitle>{title}</DialogTitle>
						<DialogDescription>
							Personal skills are available to your agents and stored as a
							single SKILL.md file with frontmatter. For richer skills with
							supporting files, add them to your repo under `.agents/skills/` or
							load them from a workspace.
						</DialogDescription>
					</DialogHeader>

					<div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-6 py-4">
						{submitError && (
							<Alert severity="error">
								<AlertTitle>{submitError.message}</AlertTitle>
								{submitError.detail && (
									<AlertDescription>{submitError.detail}</AlertDescription>
								)}
							</Alert>
						)}

						<div className="flex flex-col gap-3 rounded-md border border-border-default p-4">
							<div className="flex flex-col gap-1">
								<Label htmlFor={importId}>Import from SKILL.md</Label>
								<p className="m-0 text-xs text-content-secondary">
									Paste a full SKILL.md file with frontmatter to auto-fill the
									fields below.
								</p>
							</div>
							<TextareaAutosize
								id={importId}
								value={importContent}
								onChange={handleImportContentChange}
								onPaste={handleImportContentPaste}
								placeholder="---\nname: my-skill\ndescription: ...\n---\n\nBody..."
								disabled={isSubmitting}
								minRows={4}
								maxRows={10}
								className="w-full resize-y rounded-md border border-border bg-transparent px-3 py-2 font-mono text-sm leading-relaxed text-content-primary placeholder:text-content-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link disabled:cursor-not-allowed disabled:opacity-50"
							/>
							{importStatus && (
								<Alert severity={importStatus.kind}>
									<AlertTitle>{importStatus.title}</AlertTitle>
									{importStatus.detail && (
										<AlertDescription>{importStatus.detail}</AlertDescription>
									)}
								</Alert>
							)}
							<div className="flex justify-end gap-2">
								{importContent && (
									<Button
										variant="outline"
										size="sm"
										disabled={isSubmitting}
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
									disabled={isSubmitting || !importContent.trim()}
									onClick={() => {
										void importSkillMarkdown(importContent);
									}}
								>
									Import
								</Button>
							</div>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor={nameId}>Name</Label>
							<Input
								id={nameId}
								name="name"
								value={form.values.name}
								onChange={form.handleChange}
								onBlur={form.handleBlur}
								placeholder="review-database-query"
								readOnly={!isCreate}
								disabled={isSubmitting}
								aria-invalid={Boolean(nameError)}
								aria-describedby={nameError ? nameErrorId : undefined}
								className={cn(!isCreate && "bg-surface-secondary")}
							/>
							{nameError ? (
								<p
									id={nameErrorId}
									className="m-0 text-xs text-content-destructive"
								>
									{nameError}
								</p>
							) : (
								<p className="m-0 text-xs text-content-secondary">
									Use lowercase letters, numbers, and hyphens. Names cannot be
									changed after creation.
								</p>
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
								disabled={isSubmitting}
								aria-invalid={Boolean(descriptionError)}
								aria-describedby={
									descriptionError ? descriptionErrorId : undefined
								}
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

						<div className="flex flex-col gap-2">
							<Label htmlFor={bodyId}>Body</Label>
							<TextareaAutosize
								id={bodyId}
								name="body"
								value={form.values.body}
								onChange={form.handleChange}
								onBlur={form.handleBlur}
								placeholder="Describe when and how agents should use this skill."
								disabled={isSubmitting}
								minRows={8}
								aria-invalid={Boolean(bodyError)}
								aria-describedby={bodyError ? bodyErrorId : undefined}
								className={cn(
									"w-full resize-y rounded-md border border-border bg-transparent px-3 py-2 font-mono text-sm leading-relaxed text-content-primary placeholder:text-content-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link disabled:cursor-not-allowed disabled:opacity-50",
									bodyError && "border-border-destructive",
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
								{formatKiB(sizeBytes)} of{" "}
								{formatKiB(PERSONAL_SKILL_MAX_SIZE_BYTES)}
								used.
							</p>
						</div>
					</div>

					<DialogFooter className="border-t border-border-default px-6 py-4">
						<Button
							variant="outline"
							disabled={isSubmitting}
							onClick={() => onOpenChange(false)}
						>
							Cancel
						</Button>
						<Button
							type="submit"
							disabled={isSubmitting || !form.isValid || !form.dirty}
						>
							{isSubmitting && <Spinner className="size-4" loading />}
							{submitLabel}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};
