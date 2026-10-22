import type { EditorProps } from "@monaco-editor/react";

vi.mock("#/components/SyntaxHighlighter/coderTheme", () => ({
	useCoderTheme: () => ({ name: "coder", isLoading: false }),
}));

// Monaco's layout and input projection require a browser. Exercise form state
// with its controlled-value contract here; stories render the real editor.
vi.mock("@monaco-editor/react", () => ({
	loader: { config: vi.fn() },
	default: ({ value, onChange, options }: EditorProps) => (
		<textarea
			aria-label={options?.ariaLabel}
			value={value}
			readOnly={options?.readOnly}
			disabled={options?.tabIndex === -1}
			tabIndex={options?.tabIndex}
			onChange={(event) =>
				onChange?.(event.target.value, {
					changes: [],
					detailedReasonsChangeLengths: [],
					eol: "\n",
					versionId: 1,
					isUndoing: false,
					isRedoing: false,
					isFlush: false,
					isEolChange: false,
				})
			}
		/>
	),
}));
