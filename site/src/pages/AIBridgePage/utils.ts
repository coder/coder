export const roundTokenDisplay = (tokens: number) => {
	if (tokens >= 1000) {
		return `${(tokens / 1000).toFixed(1)}k`;
	}
	return tokens.toString();
};

export const roundDurationDisplay = (duration: number) => {
	if (duration >= 1000) {
		return `${(duration / 1000).toFixed(1)}s`;
	}
	return `${duration.toFixed(0)}ms`;
};

export const getProviderDisplayName = (provider: string) => {
	switch (provider) {
		case "anthropic":
			return "Anthropic";
		case "openai":
			return "OpenAI";
		case "copilot":
			return "Github";
		default:
			return "Unknown";
	}
};

// FIXME the current AIBridgeProviderIcon uses the claude icon for the
// anthropic provider. while it's still in use in the RequestLogsPage, we need
// to hack around it here, but when we delete that page, we can just swap the
// icon
export const getProviderIconName = (provider: string) => {
	if (provider === "anthropic") {
		return "anthropic-neue";
	}
	return provider;
};

export const prettyFormatJSON = (input: string) => {
	let formattedInput = input;

	try {
		formattedInput = JSON.stringify(JSON.parse(input), null, 2);
	} catch {
		// not JSON, use as-is
	}

	return formattedInput;
};
