import {
	LoaderIcon,
	MessageCircleQuestionIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { flushSync } from "react-dom";
import { useMutation } from "react-query";
import { Button } from "#/components/Button/Button";
import { Response } from "../Response";
import { QuestionStep } from "./QuestionStep";
import {
	filterQuestionOptions,
	formatOutgoingMessage,
	getDefaultDraft,
	getQuestionKey,
	getQuestionText,
	isDraftAnswered,
	OTHER_OPTION_VALUE,
} from "./questionAnswers";
import {
	type FormDraft,
	getQuestionCallKey,
	type QuestionDraft,
	readFormDraft,
	removeFormDraft,
	saveFormDraft,
} from "./questionDraft";
import { getFormShortcut, ShortcutHint } from "./questionShortcuts";
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

const getSubmissionErrorMessage = (error: unknown): string | undefined => {
	if (!error) {
		return undefined;
	}

	if (error instanceof Error) {
		return error.message;
	}

	return "Failed to submit your answer.";
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
	// Questions and their options can stream in after the form mounts, so a
	// question the user has not answered takes its default from its options now.
	const drafts = filteredQuestions.map((question, index) => {
		const draft = formDraft.drafts.at(index);
		return draft?.selection === undefined ? getDefaultDraft(question) : draft;
	});
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
	const draftKey =
		toolCallId === undefined
			? undefined
			: getQuestionCallKey(toolCallId, startedAt);

	// Only the latest question takes an answer, so an older one's draft goes.
	useEffect(() => {
		if (!isLatestAskUserQuestion && draftKey !== undefined) {
			removeFormDraft(draftKey);
		}
	}, [isLatestAskUserQuestion, draftKey]);

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

	return (
		<div className="w-full">
			{isRunning && runningHeader}
			<AskUserQuestionForm
				questions={questions}
				previousResponseText={previousResponseText}
				draftKey={draftKey}
				showSubmittedAnswer={isLatestAskUserQuestion}
				onSubmitAnswer={isAnswerable ? onSubmitAnswer : undefined}
			/>
		</div>
	);
};
