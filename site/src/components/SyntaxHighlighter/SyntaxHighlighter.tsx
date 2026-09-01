import { useTheme } from "@emotion/react";
import Editor, { DiffEditor, loader } from "@monaco-editor/react";
import type * as Monaco from "monaco-editor";
import * as monaco from "monaco-editor";
import {
	type ComponentProps,
	type FC,
	useCallback,
	useEffect,
	useRef,
} from "react";
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
			data-pixel="ignore"
			className="py-2 h-full"
			style={{
				backgroundColor: theme.monaco.colors["editor.background"],
			}}
		>
			{hasDiff ? (
				<DiffFile original={compareWith} modified={value} {...commonProps} />
			) : (
				<Editor value={value} {...commonProps} />
			)}
		</div>
	);
};

type DiffFileProps = CommonEditorProps & {
	original: string;
	modified: string;
};

// Renders the diff editor and owns its model cleanup. Scoping this to its own
// component means the cleanup effect runs whenever the diff editor unmounts,
// including when SyntaxHighlighter stays mounted but switches diff -> plain for
// a file that stopped changing between versions.
//
// keepCurrent{Original,Modified}Model stops @monaco-editor/react from disposing
// the models mid-teardown (which throws), so we dispose them ourselves after
// React has torn the editor down. Without this the models accumulate unbounded
// as users open template versions until the tab runs out of memory.
const DiffFile: FC<DiffFileProps> = ({
	original,
	modified,
	onMount,
	...editorProps
}) => {
	const diffModelsRef = useRef<{
		original: Monaco.editor.ITextModel;
		modified: Monaco.editor.ITextModel;
	} | null>(null);

	const handleMount = useCallback(
		(
			editor: Monaco.editor.IStandaloneDiffEditor,
			monacoInstance: typeof Monaco,
		) => {
			onMount?.(editor, monacoInstance);

			const diffModel = editor.getModel();
			diffModelsRef.current = diffModel
				? { original: diffModel.original, modified: diffModel.modified }
				: null;

			// Auto-scroll to the first diff. Diffs may already be computed by the
			// time onMount fires, so check immediately and otherwise wait for the
			// onDidUpdateDiff event.
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
		[onMount],
	);

	useEffect(() => {
		return () => {
			const models = diffModelsRef.current;
			if (!models) {
				return;
			}
			diffModelsRef.current = null;
			// Defer disposal until after React's commit finishes. @monaco-editor/
			// react disposes the diff widget in its own unmount cleanup; freeing
			// the models in the same synchronous teardown makes the widget throw
			// "TextModel got disposed before DiffEditorWidget model got reset".
			queueMicrotask(() => {
				models.original.dispose();
				models.modified.dispose();
			});
		};
	}, []);

	return (
		<DiffEditor
			original={original}
			modified={modified}
			{...editorProps}
			keepCurrentOriginalModel
			keepCurrentModifiedModel
			onMount={handleMount}
		/>
	);
};
