import { type FC, Fragment, type ReactNode } from "react";

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

// input is not guaranteed to be valid JSON, so we need to catch any errors
// and return the original string if it is not valid
export const JsonPrettyPrinter: FC<{ input: string }> = ({ input }) => {
	try {
		return formatJSONValue(JSON.parse(input), 0);
	} catch {
		return input;
	}
};
