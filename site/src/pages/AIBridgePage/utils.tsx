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

import type { ReactNode } from "react";
import { Fragment } from "react";

const formatJSONValue = (value: unknown, depth: number): ReactNode => {
	switch (typeof value) {
		case "boolean":
			return <span className="text-syntax-boolean">{String(value)}</span>;
		case "number":
			return <span className="text-syntax-number">{value}</span>;
		case "string":
			return <span className="text-syntax-string">"{value}"</span>;
		case "object":
			if (value === null) return "null";
	}
	const pad = "  ".repeat(depth);
	const inner = "  ".repeat(depth + 1);
	if (Array.isArray(value)) {
		if (value.length === 0) return "[]";
		return (
			<>
				{/* biome-ignore lint/style/useConsistentCurlyBraces: \n requires a JS string literal */}
				{"[\n"}
				{value.map((v, i) => (
					<Fragment key={i}>
						{inner}
						{formatJSONValue(v, depth + 1)}
						{i < value.length - 1 ? ",\n" : "\n"}
					</Fragment>
				))}
				{pad}]
			</>
		);
	}
	const entries = Object.entries(value as Record<string, unknown>);
	if (entries.length === 0) return "{}";
	return (
		<>
			{/* biome-ignore lint/style/useConsistentCurlyBraces: \n requires a JS string literal */}
			{"{\n"}
			{entries.map(([k, v], i) => (
				<Fragment key={k}>
					{inner}
					<span className="text-syntax-key">"{k}"</span>
					{/* biome-ignore lint/style/useConsistentCurlyBraces: keeps spacing explicit */}
					{": "}
					{formatJSONValue(v, depth + 1)}
					{i < entries.length - 1 ? ",\n" : "\n"}
				</Fragment>
			))}
			{pad}
			{"}"}
		</>
	);
};

export const prettyFormatJSON = (input: string): ReactNode => {
	try {
		return formatJSONValue(JSON.parse(input), 0);
	} catch {
		return input;
	}
};
