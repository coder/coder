import { cn } from "cn";
import {
	ChevronDownIcon,
	ChevronUpIcon,
	LoaderIcon,
	MessageCircleQuestionIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { flushSync } from "react-dom";
import { useMutation } from "react-query";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { Response } from "../Response";
import { getAnswerBody, getAsyncAnswerPrefix } from "./asyncAnswer";
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
	options: Array<{ label: string; description: string }>;
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
	/** Set for async questions; their answer names this call. */
	asyncToolCallId?: string;
	isDocked?: boolean;
	onSubmitAnswer?: (message: string) => Promise<void> | void;
};

const OTHER_OPTION_VALUE = "other";

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
	description: string;
	isEnabled: boolean;
};

const QuestionOption: React.FC<QuestionOptionProps> = ({
	id,
	value,
	label,
	description,
	isEnabled,
}) => {
	const descriptionId = `${id}-description`;
	// The description stays outside the label because its Markdown can hold
	// links and copy buttons, which must not nest in the radio's label.
	return (
		<div className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-0.5 py-1.5">
			<RadioGroupItem
				aria-describedby={descriptionId}
				className="self-center"
				disabled={!isEnabled}
				id={id}
				value={value}
			/>
			<label
				htmlFor={id}
				className={cn(
					"text-[13px] font-medium text-content-primary",
					isEnabled ? "cursor-pointer" : "cursor-default",
				)}
			>
				{label}
			</label>
			<Response
				id={descriptionId}
				className="col-start-2 text-content-secondary"
			>
				{description}
			</Response>
		</div>
	);
};

type OtherQuestionOptionProps = {
	id: string;
	questionHeader: string;
	otherText: string;
	isEnabled: boolean;
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
	isEnabled,
	onSelect,
	onTextChange,
}) => {
	return (
		<div className="grid grid-cols-[auto_1fr] items-center gap-x-3 py-1.5">
			<RadioGroupItem
				aria-label="Other"
				disabled={!isEnabled}
				id={id}
				value={OTHER_OPTION_VALUE}
			/>
			<Input
				aria-label={`Other response for ${questionHeader}`}
				className="h-8 text-[13px] md:text-[13px]"
				disabled={!isEnabled}
				placeholder="Other: type your own answer"
				value={otherText}
				onClick={onSelect}
				onChange={(event) => {
					onTextChange(event.currentTarget.value);
				}}
			/>
		</div>
	);
};

type QuestionStepProps = {
	question: AskUserQuestion;
	questionIndex: number;
	questionCount: number;
	idPrefix: string;
	draft: QuestionDraft;
	/** Off while the panel header above the form shows the label. */
	showsLabel: boolean;
	isEnabled: boolean;
	questionTextRef: React.Ref<HTMLDivElement>;
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
	showsLabel,
	isEnabled,
	questionTextRef,
	onOptionChange,
	onOtherSelect,
	onOtherTextChange,
}) => {
	const questionIdBase = `${idPrefix}-question-${questionIndex}`;
	const questionTextId = `${questionIdBase}-text`;
	const questionLabelId = `${questionIdBase}-label`;

	return (
		<div>
			{showsLabel && (
				<p
					id={questionLabelId}
					className="m-0 flex items-center gap-1.5 text-xs text-content-secondary"
				>
					<MessageCircleQuestionIcon aria-hidden="true" className="size-3.5" />
					{getQuestionLabel(question, questionIndex, questionCount)}
				</p>
			)}
			<Response
				ref={questionTextRef}
				id={questionTextId}
				tabIndex={-1}
				className="mt-1 text-[15px] font-medium focus:outline-hidden"
			>
				{getQuestionText(question)}
			</Response>
			<RadioGroup
				aria-labelledby={
					showsLabel ? `${questionLabelId} ${questionTextId}` : questionTextId
				}
				className="mt-2 gap-0"
				name={`${questionIdBase}-options`}
				value={getSelectedValue(draft)}
				onValueChange={onOptionChange}
			>
				{question.options.map((option, optionIndex) => (
					<QuestionOption
						key={`${option.label}-${option.description}-${optionIndex}`}
						id={`${questionIdBase}-option-${optionIndex}`}
						value={`option-${optionIndex}`}
						label={getOptionLabel(question, optionIndex)}
						description={option.description || "No description provided."}
						isEnabled={isEnabled}
					/>
				))}
				<OtherQuestionOption
					id={`${questionIdBase}-option-other`}
					questionHeader={getQuestionHeader(question, questionIndex)}
					otherText={draft.otherText}
					isEnabled={isEnabled}
					onSelect={onOtherSelect}
					onTextChange={onOtherTextChange}
				/>
			</RadioGroup>
		</div>
	);
};

type QuestionPanelHeaderProps = {
	label: string;
	progress: string | undefined;
	isCollapsed: boolean;
	onToggle: () => void;
};

const QuestionPanelHeader: React.FC<QuestionPanelHeaderProps> = ({
	label,
	progress,
	isCollapsed,
	onToggle,
}) => (
	<button
		type="button"
		aria-expanded={!isCollapsed}
		className="flex w-full appearance-none items-center gap-1.5 px-3 py-2.5 text-left font-sans text-xs text-content-secondary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link"
		onClick={onToggle}
	>
		<MessageCircleQuestionIcon
			aria-hidden="true"
			className="size-3.5 shrink-0"
		/>
		<span className="min-w-0 flex-1 truncate">{label}</span>
		{progress && <span className="shrink-0">{progress}</span>}
		{isCollapsed ? (
			<ChevronUpIcon aria-hidden="true" className="size-4 shrink-0" />
		) : (
			<ChevronDownIcon aria-hidden="true" className="size-4 shrink-0" />
		)}
	</button>
);

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
	/** An answer that the queue or the transcript already shows. */
	previousResponseText?: string;
	asyncToolCallId?: string;
	/** Saves the unsent answers under this key until the form shows an answer. */
	draftKey?: string;
	showSubmittedAnswer: boolean;
	/** The form is read-only without it, and once it shows an answer. */
	onSubmitAnswer?: (message: string) => Promise<void> | void;
	onStepChange?: () => void;
	/** Makes the label a header that collapses the form, which keeps its state. */
	onToggleCollapsed?: () => void;
	isCollapsed?: boolean;
	hasStickyActions?: boolean;
};

/**
 * Sends the answers to a question set as one message. With asyncToolCallId,
 * the message starts with the call's prefix line, and the form strips that
 * line from the answer it shows.
 */
export const AskUserQuestionForm: React.FC<AskUserQuestionFormProps> = ({
	questions,
	previousResponseText,
	asyncToolCallId,
	draftKey,
	showSubmittedAnswer,
	onSubmitAnswer,
	onStepChange,
	onToggleCollapsed,
	isCollapsed = false,
	hasStickyActions = false,
}) => {
	const idPrefix = useId();
	const questionTextRef = useRef<HTMLDivElement>(null);
	const filteredQuestions = questions.map(filterQuestionOptions);
	const [formDraft, setFormDraft] = useState<FormDraft>(
		() =>
			(draftKey === undefined
				? undefined
				: readFormDraft(draftKey, filteredQuestions)) ?? {
				drafts: filteredQuestions.map(getDefaultDraft),
				questionIndex: 0,
				answeredQuestionCount: 0,
			},
	);
	// Set once the queue or transcript shows the answer. After that, the sent
	// copy would bring back an answer removed from the queue, and a send error
	// would describe an answer the server took, so neither shows.
	const [hasShownAnswer, setHasShownAnswer] = useState(false);
	const submitAnswerMutation = useMutation({
		mutationFn: async (answerText: string) => {
			await onSubmitAnswer?.(
				asyncToolCallId === undefined
					? answerText
					: getAsyncAnswerPrefix(asyncToolCallId) + answerText,
			);
		},
		onSuccess: () => {
			if (draftKey !== undefined) {
				removeFormDraft(draftKey);
			}
		},
	});

	const answerBody = getAnswerBody(previousResponseText, asyncToolCallId);
	if (answerBody !== undefined && !hasShownAnswer) {
		setHasShownAnswer(true);
	}
	// An answer can arrive without this form's send, for example as a chat
	// message, so the saved draft goes once the form shows any answer.
	useEffect(() => {
		if (hasShownAnswer && draftKey !== undefined) {
			removeFormDraft(draftKey);
		}
	}, [hasShownAnswer, draftKey]);

	const sentResponseText =
		submitAnswerMutation.isSuccess && !hasShownAnswer
			? submitAnswerMutation.variables
			: undefined;
	const displayedSubmittedResponseText = answerBody ?? sentResponseText;
	const hasSubmittedResponse = displayedSubmittedResponseText !== undefined;
	const { drafts, answeredQuestionCount } = formDraft;
	const activeQuestionIndex = Math.min(
		formDraft.questionIndex,
		Math.max(filteredQuestions.length - 1, 0),
	);
	const isInteractive = Boolean(onSubmitAnswer) && !hasSubmittedResponse;
	const isSubmitting = submitAnswerMutation.isPending;
	const submitError = hasShownAnswer
		? undefined
		: getSubmissionErrorMessage(submitAnswerMutation.error);
	const canAdvanceToNextQuestion = isDraftAnswered(drafts[activeQuestionIndex]);
	const canSubmitAllAnswers = filteredQuestions.every((_, questionIndex) =>
		isDraftAnswered(drafts[questionIndex]),
	);
	const isWizard = filteredQuestions.length > 1;
	const isFinalQuestion = activeQuestionIndex >= filteredQuestions.length - 1;
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
	const panelQuestionIndex =
		visibleQuestions.length === 1 ? activeQuestionIndex : undefined;

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
	// view or disabled. flushSync renders that step first, and onStepChange
	// runs last so that the focus cannot undo the scroll position it sets.
	const changeQuestion = (nextFormDraft: FormDraft) => {
		flushSync(() => {
			updateFormDraft(nextFormDraft);
		});
		resetSubmitState();
		questionTextRef.current?.focus();
		onStepChange?.();
	};

	const advance = (nextDrafts: QuestionDraft[]) => {
		changeQuestion({
			drafts: nextDrafts,
			questionIndex: activeQuestionIndex + 1,
			// Next confirms every question before the one it opens, so Back
			// does not lower the answered count.
			answeredQuestionCount: Math.max(
				answeredQuestionCount,
				activeQuestionIndex + 1,
			),
		});
	};

	const submit = (finalDrafts: QuestionDraft[]) => {
		if (!finalDrafts.every(isDraftAnswered)) {
			return;
		}

		resetSubmitState();
		setHasShownAnswer(false);
		submitAnswerMutation.mutate(
			formatOutgoingMessage(filteredQuestions, finalDrafts),
		);
	};

	const handleBack = () => {
		changeQuestion({
			...formDraft,
			questionIndex: activeQuestionIndex - 1,
		});
	};

	const handleSkip = () => {
		const nextDrafts = draftsWithChange(activeQuestionIndex, {
			selection: "skipped",
		});
		if (isWizard && !isFinalQuestion) {
			advance(nextDrafts);
			return;
		}

		updateFormDraft({ ...formDraft, drafts: nextDrafts });
		submit(nextDrafts);
	};

	const handleFormSubmit = (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();

		if (isWizard && !isFinalQuestion) {
			if (canAdvanceToNextQuestion) {
				advance(drafts);
			}
			return;
		}

		submit(drafts);
	};

	const panelHeader = onToggleCollapsed && (
		<QuestionPanelHeader
			label={
				panelQuestionIndex === undefined
					? "Questions"
					: getQuestionLabel(
							filteredQuestions[panelQuestionIndex],
							panelQuestionIndex,
							filteredQuestions.length,
						)
			}
			progress={
				isCollapsed && isWizard
					? `${answeredQuestionCount}/${filteredQuestions.length} questions answered`
					: undefined
			}
			isCollapsed={isCollapsed}
			onToggle={onToggleCollapsed}
		/>
	);

	if (panelHeader && isCollapsed) {
		return panelHeader;
	}

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
							showsLabel={!panelHeader || panelQuestionIndex === undefined}
							isEnabled={isInteractive && !isSubmitting}
							questionTextRef={questionTextRef}
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
				<div
					className={cn(
						"flex items-center gap-2",
						// The negative margin covers the padding of the wrapper
						// below the header, so the row's background reaches the
						// panel's border.
						hasStickyActions
							? "sticky bottom-0 -mb-3 bg-surface-primary pt-3 pb-3"
							: "mt-3",
					)}
				>
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
					{isWizard && !isFinalQuestion ? (
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
				</div>
			)}
		</>
	);

	const body = isInteractive ? (
		<form onSubmit={handleFormSubmit}>{content}</form>
	) : (
		content
	);

	return panelHeader ? (
		<>
			{panelHeader}
			<div className="px-3 pb-3">{body}</div>
		</>
	) : (
		body
	);
};

/**
 * Shows an ask_user_question call in the transcript, with the async
 * question's announcement. A docked or closed question shows a notice in
 * place of the form.
 */
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
	asyncToolCallId,
	isDocked = false,
	onSubmitAnswer,
}) => {
	const isAsync = asyncToolCallId !== undefined;
	const isRunning = status === "running";
	const isCompleted = status === "completed";

	const isAnswered =
		getAnswerBody(previousResponseText, asyncToolCallId) !== undefined;
	const isAnswerable =
		(isAsync || isChatCompleted) &&
		isCompleted &&
		isLatestAskUserQuestion &&
		Boolean(onSubmitAnswer);
	const isClosed =
		isAsync && isCompleted && !isLatestAskUserQuestion && !isAnswered;

	// The agent keeps working, so the question needs its own announcement.
	// Screen readers announce only changes to a live region that already
	// exists, so the region renders before the result streams in.
	const announcement = (
		<span role="status" className="sr-only">
			{isAsync && isAnswerable && !isAnswered && "The agent asked a question."}
		</span>
	);
	const runningHeader = (
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
	);

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
				{announcement}
				{isRunning ? (
					runningHeader
				) : (
					<p className="text-[13px] italic text-content-secondary">
						No questions available.
					</p>
				)}
			</div>
		);
	}

	let notice: string | undefined;
	if (isDocked) {
		notice = "Answer in the question panel above the chat input.";
	} else if (isClosed) {
		notice = "This question is closed.";
	}

	return (
		<div className="w-full">
			{announcement}
			{isRunning && runningHeader}
			{notice === undefined ? (
				<AskUserQuestionForm
					questions={questions}
					previousResponseText={previousResponseText}
					asyncToolCallId={asyncToolCallId}
					draftKey={
						toolCallId === undefined
							? undefined
							: getQuestionCallKey(toolCallId, startedAt)
					}
					showSubmittedAnswer={isLatestAskUserQuestion}
					onSubmitAnswer={isAnswerable ? onSubmitAnswer : undefined}
				/>
			) : (
				<>
					<AskedQuestions questions={questions} />
					<p className="mt-3 text-[13px] text-content-secondary">{notice}</p>
				</>
			)}
		</div>
	);
};
