import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import { type FC, type FormEvent, useId, useState } from "react";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { cn } from "#/utils/cn";
import type { ToolStatus } from "./utils";

export type AskUserQuestion = {
	header: string;
	question: string;
	options: Array<{ label: string; description: string }>;
};

type QuestionAnswer =
	| {
			kind: "option";
			label: string;
			optionIndex: number;
	  }
	| {
			kind: "other";
			text: string;
	  };

type AskUserQuestionToolProps = {
	questions: AskUserQuestion[];
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	isChatCompleted?: boolean;
	isLatestAskUserQuestion?: boolean;
	previousResponseText?: string;
	onSubmitAnswer?: (message: string) => Promise<void> | void;
};

const OTHER_OPTION_VALUE = "other";

const getQuestionHeader = (
	question: AskUserQuestion,
	questionIndex: number,
): string => question.header || `Question ${questionIndex + 1}`;

const getQuestionText = (question: AskUserQuestion): string =>
	question.question || "No question provided.";

const filterQuestionOptions = (question: AskUserQuestion): AskUserQuestion => ({
	...question,
	options: question.options.filter(
		(option) => option.label.trim().toLowerCase() !== "other",
	),
});

const formatAnswer = (answer: QuestionAnswer): string =>
	answer.kind === "other"
		? `Other: ${answer.text.trim()}`
		: answer.label || `Option ${answer.optionIndex + 1}`;

const cloneAnswer = (
	answer: QuestionAnswer | undefined,
): QuestionAnswer | undefined => {
	if (!answer) {
		return undefined;
	}

	if (answer.kind === "other") {
		return { kind: "other", text: answer.text };
	}

	return {
		kind: "option",
		label: answer.label,
		optionIndex: answer.optionIndex,
	};
};

const isAnswerValid = (
	answer: QuestionAnswer | undefined,
): answer is QuestionAnswer => {
	if (!answer) {
		return false;
	}

	if (answer.kind === "other") {
		return answer.text.trim().length > 0;
	}

	return answer.label.trim().length > 0;
};

const getSelectedValue = (
	answer: QuestionAnswer | undefined,
): string | undefined => {
	if (!answer) {
		return undefined;
	}

	if (answer.kind === "other") {
		return OTHER_OPTION_VALUE;
	}

	return `option-${answer.optionIndex}`;
};

const formatOutgoingMessage = (
	questions: AskUserQuestion[],
	answers: readonly QuestionAnswer[],
): string => {
	if (questions.length === 1) {
		return formatAnswer(answers[0]);
	}

	return questions
		.map((question, questionIndex) => {
			return `${questionIndex + 1}. ${getQuestionHeader(question, questionIndex)}: ${formatAnswer(answers[questionIndex])}`;
		})
		.join("\n");
};

export const AskUserQuestionTool: FC<AskUserQuestionToolProps> = ({
	questions,
	status,
	isError,
	errorMessage,
	isChatCompleted = false,
	isLatestAskUserQuestion = false,
	previousResponseText,
	onSubmitAnswer,
}) => {
	const idPrefix = useId();
	const filteredQuestions = questions.map(filterQuestionOptions);
	const [answers, setAnswers] = useState<Array<QuestionAnswer | undefined>>(
		() =>
			filteredQuestions.map((question) => {
				const firstOption = question.options[0];
				if (!firstOption) {
					return undefined;
				}
				return {
					kind: "option" as const,
					label: firstOption.label || "Option 1",
					optionIndex: 0,
				};
			}),
	);
	const [currentQuestionIndex, setCurrentQuestionIndex] = useState(0);
	const [isSubmitting, setIsSubmitting] = useState(false);
	const [submitError, setSubmitError] = useState<string | undefined>();
	const [submittedResponseText, setSubmittedResponseText] = useState<
		string | null
	>(null);
	const isRunning = status === "running";
	const displayedSubmittedResponseText =
		previousResponseText ?? submittedResponseText;
	const hasSubmittedResponse = displayedSubmittedResponseText != null;
	const showAnsweredState = status === "completed" && hasSubmittedResponse;
	const showSubmittedResponse = showAnsweredState && isLatestAskUserQuestion;
	const activeQuestionIndex = Math.min(
		currentQuestionIndex,
		Math.max(filteredQuestions.length - 1, 0),
	);
	const currentAnswer = answers[activeQuestionIndex];
	const isInteractive =
		isChatCompleted &&
		status === "completed" &&
		isLatestAskUserQuestion &&
		!hasSubmittedResponse &&
		Boolean(onSubmitAnswer);
	const canAdvanceToNextQuestion = isAnswerValid(currentAnswer);
	const canSubmitAllAnswers = filteredQuestions.every((_, questionIndex) =>
		isAnswerValid(answers[questionIndex]),
	);

	const setAnswerAtIndex = (
		questionIndex: number,
		nextAnswer: QuestionAnswer | undefined,
	) => {
		setAnswers((currentAnswers) => {
			const nextAnswers = [...currentAnswers];
			nextAnswers[questionIndex] = nextAnswer;
			return nextAnswers;
		});
		setSubmitError(undefined);
	};

	const handleOptionChange = (
		questionIndex: number,
		question: AskUserQuestion,
		value: string,
	) => {
		if (value === OTHER_OPTION_VALUE) {
			const previousAnswer = answers[questionIndex];
			setAnswerAtIndex(
				questionIndex,
				previousAnswer?.kind === "other"
					? previousAnswer
					: { kind: "other", text: "" },
			);
			return;
		}

		const optionIndex = Number.parseInt(value.replace("option-", ""), 10);
		const option = question.options[optionIndex];
		if (!option) {
			return;
		}

		setAnswerAtIndex(questionIndex, {
			kind: "option",
			label: option.label || `Option ${optionIndex + 1}`,
			optionIndex,
		});
	};

	const handleBack = () => {
		setCurrentQuestionIndex((currentIndex) => {
			return Math.max(currentIndex - 1, 0);
		});
		setSubmitError(undefined);
	};

	const handleNext = () => {
		if (!canAdvanceToNextQuestion) {
			return;
		}

		setCurrentQuestionIndex((currentIndex) => {
			return Math.min(currentIndex + 1, filteredQuestions.length - 1);
		});
		setSubmitError(undefined);
	};

	const handleSubmit = async () => {
		if (!onSubmitAnswer || !isInteractive || !canSubmitAllAnswers) {
			return;
		}

		const finalizedAnswers = filteredQuestions.map((_, questionIndex) => {
			return cloneAnswer(answers[questionIndex]);
		});
		if (!finalizedAnswers.every(isAnswerValid)) {
			return;
		}

		const outgoingMessage = formatOutgoingMessage(
			filteredQuestions,
			finalizedAnswers,
		);
		setIsSubmitting(true);
		setSubmitError(undefined);
		try {
			await onSubmitAnswer(outgoingMessage);
		} catch (error) {
			setSubmitError(
				error instanceof Error
					? error.message
					: "Failed to submit your answer.",
			);
			setIsSubmitting(false);
			return;
		}

		setSubmittedResponseText(outgoingMessage);
		setIsSubmitting(false);
	};

	const handleFormSubmit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (!isInteractive) {
			return;
		}

		if (
			filteredQuestions.length > 1 &&
			activeQuestionIndex < filteredQuestions.length - 1
		) {
			handleNext();
			return;
		}

		void handleSubmit();
	};

	if (isError) {
		return (
			<div className="w-full">
				<div
					role="alert"
					className="flex items-center gap-1.5 py-0.5 text-sm text-content-secondary"
				>
					<TriangleAlertIcon
						aria-label="Error"
						className="h-3.5 w-3.5 shrink-0 text-content-secondary"
					/>
					<span>{errorMessage || "Failed to ask questions"}</span>
				</div>
			</div>
		);
	}

	if (questions.length === 0) {
		return (
			<div className="w-full">
				{isRunning ? (
					<div
						role="status"
						aria-live="polite"
						className="flex items-center gap-1.5 py-0.5"
					>
						<span className="text-sm text-content-secondary">
							Asking for clarification...
						</span>
						<LoaderIcon
							data-testid="ask-user-question-loading-icon"
							className="h-3.5 w-3.5 shrink-0 animate-spin text-content-secondary motion-reduce:animate-none"
						/>
					</div>
				) : (
					<p className="text-sm italic text-content-secondary">
						No questions available.
					</p>
				)}
			</div>
		);
	}

	const visibleQuestions =
		isInteractive && filteredQuestions.length > 1
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

	const content = (
		<>
			<div className="space-y-5">
				{visibleQuestions.map(({ question, questionIndex }) => {
					const questionHeader = getQuestionHeader(question, questionIndex);
					const questionText = getQuestionText(question);
					const questionIdBase = `${idPrefix}-question-${questionIndex}`;
					const questionHeaderId = `${questionIdBase}-header`;
					const questionTextId = `${questionIdBase}-text`;
					const answer = answers[questionIndex];
					const isOtherSelected = answer?.kind === "other";
					const optionCount = question.options.length;
					const showProgress = isInteractive && filteredQuestions.length > 1;

					if (showAnsweredState) {
						return (
							<p
								key={`${question.header}-${question.question}-${questionIndex}`}
								id={questionTextId}
								className="whitespace-pre-wrap text-sm text-content-primary"
							>
								{questionText}
							</p>
						);
					}

					return (
						<div
							key={`${question.header}-${question.question}-${questionIndex}`}
							className="space-y-3"
						>
							{showProgress && (
								<p className="text-xs font-medium text-content-secondary">
									Question {questionIndex + 1} of {filteredQuestions.length}
								</p>
							)}
							<div className="space-y-1.5">
								<p
									id={questionHeaderId}
									className="text-xs font-medium text-content-secondary"
								>
									{questionHeader}
								</p>
								<p
									id={questionTextId}
									className="whitespace-pre-wrap text-sm text-content-primary"
								>
									{questionText}
								</p>
							</div>
							<div className="rounded-md border border-solid border-border-default px-3 py-1">
								<RadioGroup
									aria-labelledby={`${questionHeaderId} ${questionTextId}`}
									className="space-y-1"
									name={`${questionIdBase}-options`}
									value={getSelectedValue(answer)}
									onValueChange={(value) => {
										handleOptionChange(questionIndex, question, value);
									}}
								>
									{question.options.map((option, optionIndex) => {
										const optionId = `${questionIdBase}-option-${optionIndex}`;

										return (
											<label
												key={`${option.label}-${option.description}-${optionIndex}`}
												htmlFor={optionId}
												className={cn(
													"grid gap-x-3 gap-y-0.5 py-1.5",
													isInteractive && !isSubmitting
														? "cursor-pointer"
														: "cursor-default",
												)}
												style={{ gridTemplateColumns: "auto 1fr" }}
											>
												<RadioGroupItem
													className="self-center"
													disabled={!isInteractive || isSubmitting}
													id={optionId}
													value={`option-${optionIndex}`}
												/>
												<span className="text-sm font-medium text-content-primary">
													{option.label || `Option ${optionIndex + 1}`}
												</span>
												<p className="col-start-2 m-0 whitespace-pre-wrap text-sm text-content-secondary">
													{option.description || "No description provided."}
												</p>
											</label>
										);
									})}
									{(() => {
										const otherOptionId = `${questionIdBase}-option-${optionCount}`;
										return (
											<div className="space-y-2">
												<label
													htmlFor={otherOptionId}
													className={cn(
														"grid gap-x-3 gap-y-0.5 py-1.5",
														isInteractive && !isSubmitting
															? "cursor-pointer"
															: "cursor-default",
													)}
													style={{ gridTemplateColumns: "auto 1fr" }}
												>
													<RadioGroupItem
														className="self-center"
														disabled={!isInteractive || isSubmitting}
														id={otherOptionId}
														value={OTHER_OPTION_VALUE}
													/>
													<span className="text-sm font-medium text-content-primary">
														Other
													</span>
													<p className="col-start-2 m-0 whitespace-pre-wrap text-sm text-content-secondary">
														Share a different answer.
													</p>
												</label>
												{isOtherSelected && (
													<div className="pl-7">
														<Input
															autoFocus={isInteractive}
															aria-label={`Other response for ${questionHeader}`}
															disabled={!isInteractive || isSubmitting}
															placeholder="Describe another answer"
															value={
																answer?.kind === "other" ? answer.text : ""
															}
															onChange={(event) => {
																setAnswerAtIndex(questionIndex, {
																	kind: "other",
																	text: event.currentTarget.value,
																});
															}}
														/>
													</div>
												)}
											</div>
										);
									})()}
								</RadioGroup>
							</div>
						</div>
					);
				})}
			</div>

			{showSubmittedResponse && (
				<div className="mt-4 rounded-md border border-solid border-border-default bg-surface-secondary px-3 py-2">
					<p className="text-xs font-medium text-content-secondary">
						Submitted answer
					</p>
					<p className="mt-1 whitespace-pre-wrap text-sm text-content-primary">
						{displayedSubmittedResponseText || "No answer recorded."}
					</p>
				</div>
			)}

			{submitError && (
				<div
					role="alert"
					className="mt-3 flex items-center gap-1.5 text-sm text-content-destructive"
				>
					<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0" />
					<span>{submitError}</span>
				</div>
			)}

			{isInteractive && (
				<div className="mt-4 flex items-center gap-2">
					{filteredQuestions.length > 1 && (
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
					{filteredQuestions.length > 1 &&
					activeQuestionIndex < filteredQuestions.length - 1 ? (
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
								<LoaderIcon className="h-3.5 w-3.5 animate-spin motion-reduce:animate-none" />
							)}
							{isSubmitting ? "Submitting..." : "Submit"}
						</Button>
					)}
				</div>
			)}
		</>
	);

	return (
		<div className="w-full">
			{isRunning && (
				<div
					role="status"
					aria-live="polite"
					className="flex items-center gap-1.5 py-0.5"
				>
					<span className="text-sm text-content-secondary">
						Asking for clarification...
					</span>
					<LoaderIcon
						data-testid="ask-user-question-loading-icon"
						className="h-3.5 w-3.5 shrink-0 animate-spin text-content-secondary motion-reduce:animate-none"
					/>
				</div>
			)}

			{isInteractive ? (
				<form onSubmit={handleFormSubmit}>{content}</form>
			) : (
				content
			)}
		</div>
	);
};
