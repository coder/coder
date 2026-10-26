import { useState } from "react";
import { useMutation } from "react-query";
import { getAsyncAnswerPrefix, getAsyncAnswerText } from "./asyncAnswer";

type AnswerSubmissionOptions = {
	asyncToolCallId: string | undefined;
	previousResponseText: string | undefined;
	onSubmitAnswer: ((message: string) => Promise<void> | void) | undefined;
};

/**
 * Sends a question's answer and tracks the answer text to show, preferring
 * the answer the queue or transcript already holds over the local copy.
 */
export const useAnswerSubmission = ({
	asyncToolCallId,
	previousResponseText,
	onSubmitAnswer,
}: AnswerSubmissionOptions) => {
	const isAsync = asyncToolCallId !== undefined;
	const [submittedResponseText, setSubmittedResponseText] = useState<
		string | null
	>(null);
	const mutation = useMutation({
		mutationFn: async (answerText: string) => {
			await onSubmitAnswer?.(
				isAsync
					? getAsyncAnswerPrefix(asyncToolCallId) + answerText
					: answerText,
			);
		},
		onSuccess: (_data, answerText) => {
			setSubmittedResponseText(answerText);
		},
	});
	const answeredResponseText = isAsync
		? getAsyncAnswerText(previousResponseText, asyncToolCallId)
		: previousResponseText;
	// An async answer can leave the queue without being delivered, so the
	// local copy only bridges the send until the queue or transcript has it.
	if (
		isAsync &&
		submittedResponseText !== null &&
		answeredResponseText !== undefined
	) {
		setSubmittedResponseText(null);
	}

	return {
		responseText: answeredResponseText ?? submittedResponseText,
		submit: mutation.mutate,
		reset: mutation.reset,
		isSubmitting: mutation.isPending,
		error: mutation.error,
	};
};
