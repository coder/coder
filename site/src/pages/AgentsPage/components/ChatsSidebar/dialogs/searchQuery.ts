const escapeChatSearchValue = (value: string): string => {
	return value.replaceAll('"', '\\"');
};

const supportedChatSearchFilterKeys = new Set([
	"archived",
	"diff_url",
	"has_unread",
	"pr_status",
]);

const splitSearchInput = (input: string): string[] => {
	const tokens: string[] = [];
	let token = "";
	let quoted = false;

	for (const character of input) {
		if (character === '"') {
			quoted = !quoted;
		}

		if (/\s/.test(character) && !quoted) {
			if (token !== "") {
				tokens.push(token);
				token = "";
			}
			continue;
		}

		token += character;
	}

	if (token !== "") {
		tokens.push(token);
	}

	return tokens;
};

const getKeyValueDelimiterIndex = (token: string): number | undefined => {
	let quoted = false;

	for (const [index, character] of [...token].entries()) {
		if (character === '"') {
			quoted = !quoted;
		}

		if (character === ":" && !quoted) {
			return index;
		}
	}

	return undefined;
};

const getKeyValuePair = (
	token: string,
): { key: string; value: string } | undefined => {
	const delimiterIndex = getKeyValueDelimiterIndex(token);
	if (
		delimiterIndex === undefined ||
		delimiterIndex === 0 ||
		delimiterIndex === token.length - 1
	) {
		return undefined;
	}

	return {
		key: token.slice(0, delimiterIndex).replaceAll('"', "").toLowerCase(),
		value: token.slice(delimiterIndex + 1).replace(/^"|"$/g, ""),
	};
};

export const normalizeChatSearchInput = (
	rawInput: string,
): string | undefined => {
	const trimmedInput = rawInput.trim();
	if (trimmedInput === "") {
		return undefined;
	}

	const tokens = splitSearchInput(trimmedInput);
	const keyValuePairs: string[] = [];
	const titleTerms: string[] = [];
	let hasFallbackTitle = false;

	for (const token of tokens) {
		const keyValuePair = getKeyValuePair(token);
		if (!keyValuePair) {
			titleTerms.push(token);
			hasFallbackTitle = true;
			continue;
		}

		if (keyValuePair.key === "title") {
			titleTerms.push(keyValuePair.value);
			continue;
		}

		if (!supportedChatSearchFilterKeys.has(keyValuePair.key)) {
			titleTerms.push(token);
			hasFallbackTitle = true;
			continue;
		}

		keyValuePairs.push(token);
	}

	if (!hasFallbackTitle) {
		return trimmedInput;
	}

	return [
		...keyValuePairs,
		`title:"${escapeChatSearchValue(titleTerms.join(" "))}"`,
	].join(" ");
};
