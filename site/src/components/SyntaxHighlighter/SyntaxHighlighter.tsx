import { useTheme } from "@emotion/react";
import Editor, { DiffEditor, loader } from "@monaco-editor/react";
import type * as Monaco from "monaco-editor";
import * as monaco from "monaco-editor";
import { type ComponentProps, type FC, useCallback } from "react";
import { useCoderTheme } from "./coderTheme";

loader.config({ monaco });

// Shared editor props with onMount typed to accept either editor variant,
// so callers don't need to know which underlying component will render.
type CommonEditorProps = Omit<
	ComponentProps<typeof Editor> & ComponentProps<typeof DiffEditor>,
	"onMount"
> & {
	onMount?: (
		editor:
			| Monaco.editor.IStandaloneCodeEditor
			| Monaco.editor.IStandaloneDiffEditor,
		monaco: typeof Monaco,
	) => void;
};

interface SyntaxHighlighterProps {
	value: string;
	language?: string;
	editorProps?: CommonEditorProps;
	compareWith?: string;
}

export const SyntaxHighlighter: FC<SyntaxHighlighterProps> = ({
	value,
	compareWith,
	language,
	editorProps,
}) => {
	const hasDiff = compareWith && value !== compareWith;
	const theme = useTheme();
	const coderTheme = useCoderTheme();

	// Auto-scroll to first diff when the diff editor mounts and diffs are computed.
	const handleDiffEditorMount = useCallback(
		(
			editor: Monaco.editor.IStandaloneDiffEditor,
			monacoInstance: typeof Monaco,
		) => {
			// Call any existing onMount handler from editorProps.
			editorProps?.onMount?.(editor, monacoInstance);

			// Diffs may already be computed by the time onMount fires,
			// so check immediately first. If not ready yet, fall back
			// to waiting for the onDidUpdateDiff event.
			const scrollToFirstDiff = () => {
				editor.goToDiff("next");
			};

			const changes = editor.getLineChanges();
			if (changes && changes.length > 0) {
				scrollToFirstDiff();
				return;
			}

			const disposable = editor.onDidUpdateDiff(() => {
				const updatedChanges = editor.getLineChanges();
				if (!updatedChanges || updatedChanges.length === 0) {
					return;
				}
				disposable.dispose();
				scrollToFirstDiff();
			});
		},
		[editorProps],
	);

	const commonProps = {
		language,
		theme: coderTheme.name,
		height: 560,
		options: {
			minimap: {
				enabled: false,
			},
			renderSideBySide: true,
			readOnly: true,
		},
		...editorProps,
	};

	if (coderTheme.isLoading) {
		return null;
	}

	return (
		<div
			data-chromatic="ignore"
			className="py-2 h-full"
			style={{
				backgroundColor: theme.monaco.colors["editor.background"],
			}}
		>
			{hasDiff ? (
				<DiffEditor
					original={compareWith}
					modified={value}
					{...commonProps}
					onMount={handleDiffEditorMount}
				/>
			) : (
				<Editor value={value} {...commonProps} />
			)}
		</div>
	);
};
