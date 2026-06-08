import Editor, { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import { type FC, useEffect } from "react";
import { MONOSPACE_FONT_FAMILY } from "#/theme/constants";

loader.config({ monaco });

let registered = false;

function registerRego() {
	if (registered) {
		return;
	}
	registered = true;

	monaco.languages.register({ id: "rego" });
	monaco.languages.setMonarchTokensProvider("rego", {
		keywords: [
			"package",
			"import",
			"default",
			"not",
			"with",
			"as",
			"if",
			"else",
			"some",
			"every",
			"in",
			"contains",
			"true",
			"false",
			"null",
		],
		tokenizer: {
			root: [
				[/#.*$/, "comment"],
				[/"(?:[^"\\]|\\.)*"/, "string"],
				[/\b\d+(\.\d+)?\b/, "number"],
				[
					/[a-zA-Z_]\w*/,
					{ cases: { "@keywords": "keyword", "@default": "identifier" } },
				],
				[/:=|==|!=|<=|>=|[=<>+\-*/%]/, "operator"],
			],
		},
	});
	monaco.languages.setLanguageConfiguration("rego", {
		comments: { lineComment: "#" },
		brackets: [
			["{", "}"],
			["[", "]"],
			["(", ")"],
		],
		autoClosingPairs: [
			{ open: "{", close: "}" },
			{ open: "[", close: "]" },
			{ open: "(", close: ")" },
			{ open: '"', close: '"' },
		],
	});
}

interface RegoEditorProps {
	value: string;
	onChange: (value: string) => void;
	ariaLabel?: string;
	height?: number;
}

export const RegoEditor: FC<RegoEditorProps> = ({
	value,
	onChange,
	ariaLabel,
	height = 240,
}) => {
	useEffect(() => {
		registerRego();
	}, []);

	return (
		<div
			className="overflow-hidden rounded border border-solid border-border"
			role="group"
			aria-label={ariaLabel}
		>
			<Editor
				height={height}
				language="rego"
				theme="vs-dark"
				value={value}
				options={{
					automaticLayout: true,
					fontFamily: MONOSPACE_FONT_FAMILY,
					fontSize: 13,
					minimap: { enabled: false },
					lineNumbers: "on",
					scrollBeyondLastLine: false,
					padding: { top: 8, bottom: 8 },
				}}
				onChange={(next) => onChange(next ?? "")}
			/>
		</div>
	);
};
