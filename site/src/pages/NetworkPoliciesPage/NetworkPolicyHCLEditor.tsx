import Editor, { loader } from "@monaco-editor/react";
import { cn } from "cn";
import * as monaco from "monaco-editor";
import { useEffect, useState } from "react";
import { useCoderTheme } from "#/components/SyntaxHighlighter/coderTheme";
import { Textarea } from "#/components/Textarea/Textarea";
import { MONOSPACE_FONT_FAMILY } from "#/theme/constants";

loader.config({ monaco });

type NetworkPolicyHCLEditorProps = Pick<
	React.ComponentProps<"textarea">,
	"id" | "aria-invalid" | "aria-describedby" | "disabled"
> & {
	name: string;
	value: string;
	readOnly: boolean;
	onChange: (value: string) => void;
	onBlur: () => void;
};

/** Adapts Monaco to a labeled form field without capturing Tab navigation. */
export const NetworkPolicyHCLEditor: React.FC<NetworkPolicyHCLEditorProps> = ({
	id,
	name,
	value,
	onChange,
	onBlur,
	disabled = false,
	readOnly,
	"aria-invalid": invalid,
	"aria-describedby": describedBy,
}) => {
	const theme = useCoderTheme();
	const [editor, setEditor] =
		useState<monaco.editor.IStandaloneCodeEditor | null>(null);

	useEffect(() => {
		// Monaco does not expose options for label targets or validation attributes.
		const input = editor?.getDomNode()?.querySelector("textarea");
		if (!input) return;
		input.id = id ?? "";
		input.name = name;
		input.disabled = disabled;
		input.setAttribute("aria-invalid", String(invalid ?? false));
		if (describedBy) {
			input.setAttribute("aria-describedby", describedBy);
		} else {
			input.removeAttribute("aria-describedby");
		}
	}, [editor, id, name, disabled, invalid, describedBy]);

	const fallback = (
		<Textarea
			id={id}
			name={name}
			value={value}
			onChange={(event) => onChange(event.target.value)}
			onBlur={theme.isLoading ? onBlur : undefined}
			disabled={disabled}
			readOnly={readOnly}
			aria-label="Policy HCL (loading editor)"
			aria-invalid={invalid}
			aria-describedby={describedBy}
			required
			spellCheck={false}
			rows={16}
			className="font-mono"
		/>
	);

	if (theme.isLoading) return fallback;

	return (
		<div
			className={cn(
				"overflow-hidden rounded-md border border-border focus-within:ring-2 focus-within:ring-content-link",
				invalid && "border-border-destructive",
				disabled && "opacity-50",
			)}
			onBlur={(event) => {
				if (!event.currentTarget.contains(event.relatedTarget)) onBlur();
			}}
		>
			<Editor
				language="hcl"
				theme={theme.name}
				value={value}
				height={360}
				loading={fallback}
				onChange={(value) => onChange(value ?? "")}
				onMount={setEditor}
				options={{
					ariaLabel: "Policy HCL",
					ariaRequired: true,
					automaticLayout: true,
					editContext: false,
					fontFamily: MONOSPACE_FONT_FAMILY,
					fontSize: 14,
					minimap: { enabled: false },
					// Pending occurrence requests reject when Monaco is disposed during navigation.
					occurrencesHighlight: "off",
					padding: { top: 12, bottom: 12 },
					readOnly: disabled || readOnly,
					domReadOnly: disabled || readOnly,
					scrollBeyondLastLine: false,
					tabFocusMode: true,
					tabIndex: disabled ? -1 : 0,
					wordWrap: "on",
				}}
			/>
		</div>
	);
};
