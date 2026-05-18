type ChatSideQuestionCommand =
	| { kind: "sideQuestion"; question: string }
	| { kind: "normal"; prompt: string }
	| { kind: "invalid" };

const commandPrefix = "/btw";
const escapedCommandPrefix = "//btw";

export const parseChatSideQuestionCommand = (
	prompt: string,
): ChatSideQuestionCommand => {
	const trimmedStart = prompt.trimStart();
	if (trimmedStart.startsWith(escapedCommandPrefix)) {
		return { kind: "normal", prompt: trimmedStart.slice(1) };
	}
	if (trimmedStart === commandPrefix) {
		return { kind: "invalid" };
	}
	if (!trimmedStart.startsWith(`${commandPrefix} `)) {
		return { kind: "normal", prompt };
	}
	const question = trimmedStart.slice(commandPrefix.length).trim();
	if (question === "") {
		return { kind: "invalid" };
	}
	return { kind: "sideQuestion", question };
};
