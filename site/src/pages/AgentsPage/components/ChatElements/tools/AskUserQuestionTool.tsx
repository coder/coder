import { cn } from "cn";
import {
	CheckIcon,
	CornerDownLeftIcon,
	LoaderIcon,
	MessageCircleQuestionIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { RadioGroup as RadioGroupPrimitive } from "radix-ui";
import { useEffect, useId, useRef, useState } from "react";
import { flushSync } from "react-dom";
import { useMutation } from "react-query";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Kbd, KbdGroup } from "#/components/Kbd/Kbd";
import { RadioGroup } from "#/components/RadioGroup/RadioGroup";
import { Response } from "../Response";
import {
	type FormDraft,
	getQuestionCallKey,
	type QuestionDraft,
	readFormDraft,
	removeFormDraft,
	saveFormDraft,
} from "./questionDraft";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

export type AskUserQuestion = {
	header: string;
	question: string;
	// Older calls, and models that ignore the schema, still send a description.
	options: Array<{ label: string; description?: string }>;
};

type AskUserQuestionToolProps = {
	questions: AskUserQuestion[];
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	isChatCompleted?: boolean;
	isLatestAskUserQuestion?: boolean;
	previousResponseText?: string;
	toolCallId?: string;
	startedAt?: string;
	onSubmitAnswer?: (message: string) => Promise<void> | void;
};

const OTHER_OPTION_VALUE = "other";
const OTHER_PLACEHOLDER = "Something else: type your own answer";

const getQuestionHeader = (
	question: AskUserQuestion,
	questionIndex: number,
): string => question.header || `Question ${questionIndex + 1}`;

const getQuestionLabel = (
	question: AskUserQuestion,
	questionIndex: number,
	questionCount: number,
): string => {
	const progress =
		questionCount > 1
			? `Question ${questionIndex + 1} of ${questionCount}`
			: "";
	return [progress, question.header].filter(Boolean).join(" · ") || "Question";
};

const getQuestionText = (question: AskUserQuestion): string =>
	question.question || "No question provided.";

const getQuestionKey = (
	question: AskUserQuestion,
	questionIndex: number,
): string => `${question.header}-${question.question}-${questionIndex}`;

const filterQuestionOptions = (question: AskUserQuestion): AskUserQuestion => ({
	...question,
	options: question.options.filter(
		(option) => option.label.trim().toLowerCase() !== "other",
	),
});

const getOptionLabel = (question: AskUserQuestion, optionIndex: number) =>
	question.options[optionIndex].label || `Option ${optionIndex + 1}`;

const getDefaultDraft = (question: AskUserQuestion): QuestionDraft => ({
	selection: question.options.length > 0 ? 0 : undefined,
	otherText: "",
});

type AnsweredDraft = QuestionDraft & {
	selection: Exclude<QuestionDraft["selection"], undefined>;
};

const isDraftAnswered = (draft: QuestionDraft): draft is AnsweredDraft => {
	if (draft.selection === undefined) {
		return false;
	}
	if (draft.selection === "other") {
		return draft.otherText.trim().length > 0;
	}
	return true;
};

const formatAnswer = (
	question: AskUserQuestion,
	draft: AnsweredDraft,
): string => {
	if (draft.selection === "other") {
		return `Other: ${draft.otherText.trim()}`;
	}
	if (draft.selection === "skipped") {
		return "Skipped";
	}
	return getOptionLabel(question, draft.selection);
};

const getSelectedValue = (draft: QuestionDraft): string => {
	if (typeof draft.selection === "number") {
		return `option-${draft.selection}`;
	}
	// An empty value keeps the radio group controlled with nothing selected.
	return draft.selection === "other" ? OTHER_OPTION_VALUE : "";
};

const formatOutgoingMessage = (
	questions: AskUserQuestion[],
	drafts: readonly AnsweredDraft[],
): string => {
	if (questions.length === 1) {
		return formatAnswer(questions[0], drafts[0]);
	}

	return questions
		.map((question, questionIndex) => {
			return `${questionIndex + 1}. ${getQuestionHeader(question, questionIndex)}: ${formatAnswer(question, drafts[questionIndex])}`;
		})
		.join("\n");
};

const getSubmissionErrorMessage = (error: unknown): string | undefined => {
	if (!error) {
		return undefined;
	}

	if (error instanceof Error) {
		return error.message;
	}

	return "Failed to submit your answer.";
};

type QuestionOptionProps = {
	id: string;
	value: string;
	label: string;
	description?: string;
	shortcut: number;
	isSelected: boolean;
	isEnabled: boolean;
	ref: React.Ref<HTMLButtonElement>;
};

type OptionRowProps = {
	className?: string;
	isSelected: boolean;
	isEnabled: boolean;
	children: React.ReactNode;
};

/** The radios hide their focus outline and the Other field drops its ring, so the row shows focus. */
const OptionRow: React.FC<OptionRowProps> = ({
	className,
	isSelected,
	isEnabled,
	children,
}) => (
	<div
		className={cn(
			"rounded-lg border border-solid transition-colors",
			"has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-content-link",
			isSelected
				? "border-surface-invert-primary bg-surface-secondary"
				: cn("border-border", isEnabled && "hover:bg-surface-secondary/50"),
			className,
		)}
	>
		{children}
	</div>
);

type ShortcutBadgeProps = {
	shortcut: number;
	isSelected: boolean;
};

const ShortcutBadge: React.FC<ShortcutBadgeProps> = ({
	shortcut,
	isSelected,
}) => (
	<Kbd
		aria-hidden="true"
		className={cn(
			"shrink-0",
			isSelected && "bg-surface-invert-primary text-content-invert",
		)}
	>
		{shortcut}
	</Kbd>
);

const QuestionOption: React.FC<QuestionOptionProps> = ({
	id,
	value,
	label,
	description,
	shortcut,
	isSelected,
	isEnabled,
	ref,
}) => {
	const descriptionId = description ? `${id}-description` : undefined;
	return (
		<OptionRow isSelected={isSelected} isEnabled={isEnabled}>
			{/* The whole first line is the radio, so a click anywhere on it selects. */}
			<RadioGroupPrimitive.Item
				ref={ref}
				id={id}
				value={value}
				disabled={!isEnabled}
				aria-describedby={descriptionId}
				aria-keyshortcuts={String(shortcut)}
				className={cn(
					"flex w-full items-start gap-3 border-0 bg-transparent p-3 text-left font-sans text-sm font-normal text-content-primary",
					"focus-visible:outline-hidden disabled:cursor-default",
					description && "pb-0.5",
					isEnabled && "cursor-pointer",
				)}
			>
				<ShortcutBadge shortcut={shortcut} isSelected={isSelected} />
				<span className="min-w-0 flex-1 wrap-anywhere">{label}</span>
				{isSelected && (
					<CheckIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
				)}
			</RadioGroupPrimitive.Item>
			{/* Outside the radio: Markdown links and copy buttons must not nest in a button. */}
			{description && (
				<Response
					id={descriptionId}
					className="pr-3 pb-3 pl-11 wrap-anywhere text-content-secondary"
				>
					{description}
				</Response>
			)}
		</OptionRow>
	);
};

type OtherQuestionOptionProps = {
	id: string;
	questionHeader: string;
	otherText: string;
	shortcut: number;
	isSelected: boolean;
	isEnabled: boolean;
	textRef: React.Ref<HTMLInputElement>;
	onSelect: () => void;
	onTextChange: (text: string) => void;
};

/**
 * Clicking the field or typing in it selects "Other". Tabbing through it
 * does not, so a keyboard user's choice survives the way to Submit.
 */
const OtherQuestionOption: React.FC<OtherQuestionOptionProps> = ({
	id,
	questionHeader,
	otherText,
	shortcut,
	isSelected,
	isEnabled,
	textRef,
	onSelect,
	onTextChange,
}) => {
	return (
		<OptionRow
			className="flex items-center gap-3 py-1.5 pr-1.5 pl-3"
			isSelected={isSelected}
			isEnabled={isEnabled}
		>
			<RadioGroupPrimitive.Item
				id={id}
				value={OTHER_OPTION_VALUE}
				disabled={!isEnabled}
				aria-label="Other"
				aria-keyshortcuts={String(shortcut)}
				className={cn(
					"shrink-0 rounded-sm border-0 bg-transparent p-0 focus-visible:outline-hidden",
					isEnabled && "cursor-pointer",
				)}
			>
				<ShortcutBadge shortcut={shortcut} isSelected={isSelected} />
			</RadioGroupPrimitive.Item>
			<Input
				ref={textRef}
				aria-label={`${OTHER_PLACEHOLDER} for ${questionHeader}`}
				className="h-8 border-0 px-0 text-sm shadow-none focus-visible:ring-0"
				disabled={!isEnabled}
				placeholder={OTHER_PLACEHOLDER}
				value={otherText}
				onClick={onSelect}
				onChange={(event) => {
					onTextChange(event.currentTarget.value);
				}}
			/>
		</OptionRow>
	);
};

type FormShortcut = { kind: "submit" } | { kind: "select"; rowNumber: number };

/** Modifier chords, IME composition and keys typed in the Other field keep their normal meaning. */
const getFormShortcut = (
	event: React.KeyboardEvent,
	rowCount: number,
): FormShortcut | undefined => {
	const { target } = event;
	if (!(target instanceof HTMLElement) || target.closest("input")) {
		return undefined;
	}
	if (
		event.altKey ||
		event.ctrlKey ||
		event.metaKey ||
		event.nativeEvent.isComposing
	) {
		return undefined;
	}

	// Radix prevents Enter's default action on a radio, so the form submits.
	if (event.key === "Enter") {
		return target.getAttribute("role") === "radio"
			? { kind: "submit" }
			: undefined;
	}

	if (!/^[1-9]$/.test(event.key)) {
		return undefined;
	}
	const rowNumber = Number(event.key);
	return rowNumber <= rowCount ? { kind: "select", rowNumber } : undefined;
};

type ShortcutHintProps = {
	rowCount: number;
	enterAction: "next" | "submit";
};

/** Hidden on narrow screens and touch-only devices, which have no keys to press. */
const ShortcutHint: React.FC<ShortcutHintProps> = ({
	rowCount,
	enterAction,
}) => (
	<KbdGroup
		aria-hidden="true"
		className="ml-auto hidden items-center gap-1.5 font-sans text-xs text-content-secondary pointer-fine:sm:inline-flex"
	>
		<Kbd>1</Kbd>to<Kbd>{rowCount}</Kbd>choose
		<Kbd className="ml-1.5">
			<CornerDownLeftIcon />
		</Kbd>
		{enterAction}
	</KbdGroup>
);

type QuestionStepProps = {
	question: AskUserQuestion;
	questionIndex: number;
	questionCount: number;
	idPrefix: string;
	draft: QuestionDraft;
	isEnabled: boolean;
	questionTextRef: React.Ref<HTMLDivElement>;
	optionRefs: React.RefObject<Array<HTMLButtonElement | null>>;
	otherTextRef: React.Ref<HTMLInputElement>;
	onOptionChange: (value: string) => void;
	onOtherSelect: () => void;
	onOtherTextChange: (text: string) => void;
};

const QuestionStep: React.FC<QuestionStepProps> = ({
	question,
	questionIndex,
	questionCount,
	idPrefix,
	draft,
	isEnabled,
	questionTextRef,
	optionRefs,
	otherTextRef,
	onOptionChange,
	onOtherSelect,
	onOtherTextChange,
}) => {
	const questionIdBase = `${idPrefix}-question-${questionIndex}`;
	const questionTextId = `${questionIdBase}-text`;
	const questionLabelId = `${questionIdBase}-label`;

	return (
		<div>
			<p
				id={questionLabelId}
				className="m-0 flex items-center gap-1.5 text-xs text-content-secondary"
			>
				<MessageCircleQuestionIcon aria-hidden="true" className="size-3.5" />
				{getQuestionLabel(question, questionIndex, questionCount)}
			</p>
			<Response
				ref={questionTextRef}
				id={questionTextId}
				tabIndex={-1}
				className="mt-1 text-[15px] font-medium focus:outline-hidden"
			>
				{getQuestionText(question)}
			</Response>
			<RadioGroup
				aria-labelledby={`${questionLabelId} ${questionTextId}`}
				className="mt-4 gap-1.5"
				name={`${questionIdBase}-options`}
				value={getSelectedValue(draft)}
				onValueChange={onOptionChange}
			>
				{question.options.map((option, optionIndex) => (
					<QuestionOption
						key={`${option.label}-${optionIndex}`}
						ref={(element) => {
							optionRefs.current[optionIndex] = element;
						}}
						id={`${questionIdBase}-option-${optionIndex}`}
						value={`option-${optionIndex}`}
						label={getOptionLabel(question, optionIndex)}
						description={option.description}
						shortcut={optionIndex + 1}
						isSelected={draft.selection === optionIndex}
						isEnabled={isEnabled}
					/>
				))}
				<OtherQuestionOption
					id={`${questionIdBase}-option-other`}
					questionHeader={getQuestionHeader(question, questionIndex)}
					otherText={draft.otherText}
					shortcut={question.options.length + 1}
					isSelected={draft.selection === "other"}
					isEnabled={isEnabled}
					textRef={otherTextRef}
					onSelect={onOtherSelect}
					onTextChange={onOtherTextChange}
				/>
			</RadioGroup>
		</div>
	);
};

type AskedQuestionsProps = {
	questions: AskUserQuestion[];
};

const AskedQuestions: React.FC<AskedQuestionsProps> = ({ questions }) => {
	return (
		<div className="space-y-4">
			{questions.map((question, questionIndex) => (
				<div
					key={getQuestionKey(question, questionIndex)}
					className="flex items-start gap-1.5 text-content-secondary"
				>
					<MessageCircleQuestionIcon
						aria-hidden="true"
						className="mt-0.5 size-4 shrink-0"
					/>
					<div className="flex min-w-0 flex-1 gap-1 text-[13px]">
						<span aria-hidden="true" className="leading-relaxed">
							Asked:
						</span>
						<Response className="min-w-0 flex-1 text-content-secondary">
							{getQuestionText(question)}
						</Response>
					</div>
				</div>
			))}
		</div>
	);
};

type AskUserQuestionFormProps = {
	questions: AskUserQuestion[];
	/** An answer that the transcript already shows. */
	previousResponseText?: string;
	/** Saves the unsent answers under this key until the form shows an answer. */
	draftKey?: string;
	showSubmittedAnswer: boolean;
	/** The form is read-only without it, and once it shows an answer. */
	onSubmitAnswer?: (message: string) => Promise<void> | void;
};

/** Sends the answers to a question set as one message. */
const AskUserQuestionForm: React.FC<AskUserQuestionFormProps> = ({
	questions,
	previousResponseText,
	draftKey,
	showSubmittedAnswer,
	onSubmitAnswer,
}) => {
	const idPrefix = useId();
	const questionTextRef = useRef<HTMLDivElement>(null);
	// Only one question is answerable at a time, so one set of refs is enough.
	const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
	const otherTextRef = useRef<HTMLInputElement>(null);
	const filteredQuestions = questions.map(filterQuestionOptions);
	const [formDraft, setFormDraft] = useState<FormDraft>(
		() =>
			(draftKey === undefined
				? undefined
				: readFormDraft(draftKey, filteredQuestions)) ?? {
				drafts: filteredQuestions.map(getDefaultDraft),
				questionIndex: 0,
			},
	);
	const submitAnswerMutation = useMutation({
		mutationFn: async (answerText: string) => {
			await onSubmitAnswer?.(answerText);
		},
		onSuccess: () => {
			if (draftKey !== undefined) {
				removeFormDraft(draftKey);
			}
		},
	});

	// An answer can arrive without this form's send, for example as a chat
	// message, so the saved draft goes once the form shows any answer.
	const hasShownAnswer = previousResponseText !== undefined;
	useEffect(() => {
		if (hasShownAnswer && draftKey !== undefined) {
			removeFormDraft(draftKey);
		}
	}, [hasShownAnswer, draftKey]);

	const sentResponseText = submitAnswerMutation.isSuccess
		? submitAnswerMutation.variables
		: undefined;
	const displayedSubmittedResponseText =
		previousResponseText ?? sentResponseText;
	const hasSubmittedResponse = displayedSubmittedResponseText !== undefined;
	// A streamed question set can grow after the form mounts.
	const drafts = filteredQuestions.map(
		(question, index) => formDraft.drafts[index] ?? getDefaultDraft(question),
	);
	const activeQuestionIndex = Math.min(
		formDraft.questionIndex,
		Math.max(filteredQuestions.length - 1, 0),
	);
	const isInteractive = Boolean(onSubmitAnswer) && !hasSubmittedResponse;
	const isSubmitting = submitAnswerMutation.isPending;
	const submitError = getSubmissionErrorMessage(submitAnswerMutation.error);
	const canAdvanceToNextQuestion = isDraftAnswered(drafts[activeQuestionIndex]);
	const canSubmitAllAnswers = filteredQuestions.every((_, questionIndex) =>
		isDraftAnswered(drafts[questionIndex]),
	);
	const isWizard = filteredQuestions.length > 1;
	const isFinalQuestion = activeQuestionIndex >= filteredQuestions.length - 1;
	const isNextStep = isWizard && !isFinalQuestion;
	const visibleQuestions =
		isInteractive && isWizard
			? [
					{
						question: filteredQuestions[activeQuestionIndex],
						questionIndex: activeQuestionIndex,
					},
				]
			: filteredQuestions.map((question, questionIndex) => ({
					question,
					questionIndex,
				}));

	const resetSubmitState = () => {
		submitAnswerMutation.reset();
	};

	const updateFormDraft = (nextFormDraft: FormDraft) => {
		setFormDraft(nextFormDraft);
		if (draftKey !== undefined) {
			saveFormDraft(draftKey, filteredQuestions, nextFormDraft);
		}
	};

	const draftsWithChange = (
		questionIndex: number,
		change: Partial<QuestionDraft>,
	): QuestionDraft[] =>
		drafts.map((draft, index) =>
			index === questionIndex ? { ...draft, ...change } : draft,
		);

	const changeDraft = (
		questionIndex: number,
		change: Partial<QuestionDraft>,
	) => {
		updateFormDraft({
			...formDraft,
			drafts: draftsWithChange(questionIndex, change),
		});
		resetSubmitState();
	};

	const handleOptionChange = (
		questionIndex: number,
		question: AskUserQuestion,
		value: string,
	) => {
		if (value === OTHER_OPTION_VALUE) {
			selectOther(questionIndex);
			return;
		}

		const optionIndex = Number.parseInt(value.replace("option-", ""), 10);
		if (!question.options[optionIndex]) {
			return;
		}

		changeDraft(questionIndex, { selection: optionIndex });
	};

	const selectOther = (questionIndex: number) => {
		if (drafts[questionIndex].selection !== "other") {
			changeDraft(questionIndex, { selection: "other" });
		}
	};

	// Focus moves to the new question, since Back or Next can end up out of
	// view or disabled. flushSync renders that step first.
	const changeQuestion = (nextFormDraft: FormDraft) => {
		flushSync(() => {
			updateFormDraft(nextFormDraft);
		});
		resetSubmitState();
		questionTextRef.current?.focus();
	};

	const advance = (nextDrafts: QuestionDraft[]) => {
		changeQuestion({
			drafts: nextDrafts,
			questionIndex: activeQuestionIndex + 1,
		});
	};

	const submit = (finalDrafts: QuestionDraft[]) => {
		if (!finalDrafts.every(isDraftAnswered)) {
			return;
		}

		resetSubmitState();
		submitAnswerMutation.mutate(
			formatOutgoingMessage(filteredQuestions, finalDrafts),
		);
	};

	const handleBack = () => {
		changeQuestion({
			...formDraft,
			drafts,
			questionIndex: activeQuestionIndex - 1,
		});
	};

	const handleSkip = () => {
		const nextDrafts = draftsWithChange(activeQuestionIndex, {
			selection: "skipped",
		});
		if (isNextStep) {
			advance(nextDrafts);
			return;
		}

		updateFormDraft({ ...formDraft, drafts: nextDrafts });
		submit(nextDrafts);
	};

	const handleFormSubmit = (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();

		if (isNextStep) {
			if (canAdvanceToNextQuestion) {
				advance(drafts);
			}
			return;
		}

		submit(drafts);
	};

	// On the form, so number keys never fire from the chat input.
	const handleFormKeyDown = (event: React.KeyboardEvent<HTMLFormElement>) => {
		if (isSubmitting) {
			return;
		}

		const question = filteredQuestions[activeQuestionIndex];
		const shortcut = getFormShortcut(event, question.options.length + 1);
		if (!shortcut) {
			return;
		}

		event.preventDefault();
		if (shortcut.kind === "submit") {
			event.currentTarget.requestSubmit();
			return;
		}

		const optionIndex = shortcut.rowNumber - 1;
		if (optionIndex < question.options.length) {
			handleOptionChange(
				activeQuestionIndex,
				question,
				`option-${optionIndex}`,
			);
			optionRefs.current[optionIndex]?.focus();
			return;
		}

		selectOther(activeQuestionIndex);
		otherTextRef.current?.focus();
	};

	const content = (
		<>
			{hasSubmittedResponse ? (
				<AskedQuestions questions={filteredQuestions} />
			) : (
				<div className="space-y-5">
					{visibleQuestions.map(({ question, questionIndex }) => (
						<QuestionStep
							key={getQuestionKey(question, questionIndex)}
							question={question}
							questionIndex={questionIndex}
							questionCount={filteredQuestions.length}
							idPrefix={idPrefix}
							draft={drafts[questionIndex]}
							isEnabled={isInteractive && !isSubmitting}
							questionTextRef={questionTextRef}
							optionRefs={optionRefs}
							otherTextRef={otherTextRef}
							onOptionChange={(value) => {
								handleOptionChange(questionIndex, question, value);
							}}
							onOtherSelect={() => {
								selectOther(questionIndex);
							}}
							onOtherTextChange={(text) => {
								changeDraft(questionIndex, {
									selection: "other",
									otherText: text,
								});
							}}
						/>
					))}
				</div>
			)}

			{showSubmittedAnswer && hasSubmittedResponse && (
				<div className="mt-4 rounded-md border border-solid border-border bg-surface-secondary px-3 py-2">
					<p className="text-xs font-medium text-content-secondary">
						Submitted answer
					</p>
					<p className="mt-1 whitespace-pre-wrap text-[13px] text-content-primary">
						{displayedSubmittedResponseText || "No answer recorded."}
					</p>
				</div>
			)}

			{submitError && (
				<div
					role="alert"
					className="mt-3 flex items-center gap-1.5 text-[13px] text-content-destructive"
				>
					<TriangleAlertIcon className="size-3.5 shrink-0" />
					<span>{submitError}</span>
				</div>
			)}

			{isInteractive && (
				<div className="mt-3 flex items-center gap-2">
					{isWizard && (
						<Button
							type="button"
							size="sm"
							variant="outline"
							onClick={handleBack}
							disabled={activeQuestionIndex === 0 || isSubmitting}
						>
							Back
						</Button>
					)}
					{isNextStep ? (
						<Button
							type="submit"
							size="sm"
							variant="outline"
							disabled={!canAdvanceToNextQuestion || isSubmitting}
						>
							Next
						</Button>
					) : (
						<Button
							type="submit"
							size="sm"
							variant="outline"
							disabled={!canSubmitAllAnswers || isSubmitting}
						>
							{isSubmitting && (
								<LoaderIcon className="size-3.5 animate-spin motion-reduce:animate-none" />
							)}
							{isSubmitting ? "Submitting..." : "Submit"}
						</Button>
					)}
					<Button
						type="button"
						size="sm"
						variant="subtle"
						onClick={handleSkip}
						disabled={isSubmitting}
					>
						Skip
					</Button>
					{!isSubmitting && (
						<ShortcutHint
							rowCount={
								filteredQuestions[activeQuestionIndex].options.length + 1
							}
							enterAction={isNextStep ? "next" : "submit"}
						/>
					)}
				</div>
			)}
		</>
	);

	return isInteractive ? (
		// oxlint-disable-next-line jsx-a11y/no-noninteractive-element-interactions -- Takes keys from the radios and the focused question text; the radios announce them with aria-keyshortcuts.
		<form onSubmit={handleFormSubmit} onKeyDown={handleFormKeyDown}>
			{content}
		</form>
	) : (
		content
	);
};

/** Shows an ask_user_question call in the transcript. */
export const AskUserQuestionTool: React.FC<AskUserQuestionToolProps> = ({
	questions,
	status,
	isError,
	errorMessage,
	isChatCompleted = false,
	isLatestAskUserQuestion = false,
	previousResponseText,
	toolCallId,
	startedAt,
	onSubmitAnswer,
}) => {
	const isRunning = status === "running";
	const isAnswerable =
		isChatCompleted &&
		status === "completed" &&
		isLatestAskUserQuestion &&
		Boolean(onSubmitAnswer);

	if (isError) {
		return (
			<div className="w-full" role="alert">
				<ToolCall.Root
					status={status}
					isError
					errorMessage={errorMessage || "Failed to ask questions"}
					hasContent={false}
				>
					<ToolCall.Header
						iconName="ask_user_question"
						label={errorMessage || "Failed to ask questions"}
					/>
				</ToolCall.Root>
			</div>
		);
	}

	if (questions.length === 0) {
		return (
			<div className="w-full">
				{isRunning ? (
					<ToolCall.Root
						status={status}
						hasContent={false}
						role="status"
						aria-live="polite"
					>
						<ToolCall.Header
							iconName="ask_user_question"
							label="Asking for clarification..."
						/>
					</ToolCall.Root>
				) : (
					<p className="text-[13px] italic text-content-secondary">
						No questions available.
					</p>
				)}
			</div>
		);
	}

	return (
		<div className="w-full">
			<AskUserQuestionForm
				questions={questions}
				previousResponseText={previousResponseText}
				draftKey={
					toolCallId === undefined
						? undefined
						: getQuestionCallKey(toolCallId, startedAt)
				}
				showSubmittedAnswer={isLatestAskUserQuestion}
				onSubmitAnswer={isAnswerable ? onSubmitAnswer : undefined}
			/>
		</div>
	);
};
