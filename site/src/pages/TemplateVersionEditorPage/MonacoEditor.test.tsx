import type { EditorProps } from "@monaco-editor/react";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as monaco from "monaco-editor";
import { useState } from "react";
import { MockTemplateResourceFiles } from "#/testHelpers/templateResources";
import { getFileText, updateFile } from "#/utils/filetree";
import { MonacoEditor } from "./MonacoEditor";

vi.mock("#/theme/context", () => ({
	useTheme: () => ({
		monaco: { base: "vs-dark", inherit: true, rules: [], colors: {} },
	}),
}));

const callbacks = vi.hoisted(() => {
	vi.stubGlobal(
		"navigator",
		new Proxy(navigator, {
			get(target, key) {
				if (key === "userAgent") return "Chrome/130.0.0.0";
				return Reflect.get(target, key, target);
			},
		}),
	);
	return new Map<string, (value: string | undefined) => void>();
});

// Keep real Monaco models and reproduce monaco-react's URI lookup, initial-value
// skip, path-before-value effects, and active-model-only disposal. Only its DOM
// projection is replaced because JSDOM cannot lay out a standalone editor.
vi.mock("@monaco-editor/react", async () => {
	const { useEffect, useRef, useState } = await import("react");
	const monaco = await import("monaco-editor");
	const getModel = (path: string, value: string) =>
		monaco.editor.getModel(monaco.Uri.parse(path)) ??
		monaco.editor.createModel(value, undefined, monaco.Uri.parse(path));
	type MockEditorProps = Pick<EditorProps, "path" | "value"> & {
		onChange?: (value: string | undefined) => void;
		onMount?: (
			editor: Pick<
				monaco.editor.IStandaloneCodeEditor,
				"getModel" | "updateOptions"
			> & {
				_standaloneKeybindingService: { addDynamicKeybinding: () => void };
			},
		) => void;
	};
	return {
		loader: { config: vi.fn() },
		default: function MockEditor({
			path = "untitled",
			value = "",
			onChange,
			onMount,
		}: MockEditorProps) {
			const [model, setModel] = useState(() => getModel(path, value));
			const current = useRef(model);
			const [mount] = useState(() => onMount);
			const initial = useRef(true);
			const suppressChange = useRef(false);
			const [, refresh] = useState(0);
			useEffect(() => {
				mount?.({
					getModel: () => current.current,
					updateOptions: () => {},
					_standaloneKeybindingService: { addDynamicKeybinding: () => {} },
				});
				return () => current.current.dispose();
			}, [mount]);
			useEffect(() => {
				current.current = getModel(path, value);
				setModel(current.current);
			}, [path, value]);
			useEffect(() => {
				if (initial.current) {
					initial.current = false;
					return;
				}
				if (value !== current.current.getValue()) {
					suppressChange.current = true;
					current.current.setValue(value);
					suppressChange.current = false;
					refresh((value) => value + 1);
				}
			}, [value]);
			useEffect(() => {
				if (onChange)
					callbacks.set(monaco.Uri.parse(path).toString(), onChange);
				const subscription = model.onDidChangeContent(() => {
					refresh((value) => value + 1);
					if (!suppressChange.current) onChange?.(current.current.getValue());
				});
				return () => subscription.dispose();
			}, [model, onChange, path]);
			return (
				<textarea
					aria-label="Editor content"
					value={model.getValue()}
					onChange={(event) => {
						model.pushStackElement();
						model.pushEditOperations(
							null,
							[{ range: model.getFullModelRange(), text: event.target.value }],
							() => null,
						);
						model.pushStackElement();
					}}
				/>
			);
		},
	};
});

beforeEach(() => {
	vi.stubGlobal(
		"document",
		new Proxy(document, {
			get(target, key) {
				if (key === "fonts") return { ready: Promise.resolve() };
				return Reflect.get(target, key, target);
			},
		}),
	);
});
afterEach(() => {
	for (const model of monaco.editor.getModels()) model.dispose();
	callbacks.clear();
	vi.unstubAllGlobals();
});

it("isolates identical paths across editor sessions and releases every owned model", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	const otherChange = vi.fn();
	const other = render(
		<MonacoEditor
			path="main.tf"
			value="another template"
			onChange={otherChange}
		/>,
	);
	const view = render(
		<MonacoEditor
			key="workspace"
			path="main.tf"
			value="workspace root"
			onChange={onChange}
		/>,
	);
	view.rerender(
		<MonacoEditor
			key="workspace"
			path="outputs.tf"
			value="workspace outputs"
			onChange={onChange}
		/>,
	);
	view.rerender(
		<MonacoEditor
			key="template"
			path="main.tf"
			value="template root"
			onChange={onChange}
		/>,
	);
	await user.click(
		within(view.container).getByRole("textbox", { name: "Editor content" }),
	);
	await user.keyboard("{End}!");
	expect(onChange).toHaveBeenLastCalledWith("template root!");
	expect(otherChange).not.toHaveBeenCalled();
	view.unmount();
	await user.click(
		within(other.container).getByRole("textbox", { name: "Editor content" }),
	);
	await user.keyboard("{End}!");
	expect(otherChange).toHaveBeenLastCalledWith("another template!");
	other.unmount();
	expect(monaco.editor.getModels()).toEqual([]);
});

it("keeps full paths, draft models, and undo history while rejecting stale change callbacks", async () => {
	const user = userEvent.setup();
	const changed = vi.fn();
	function EditorFiles() {
		const [files, setFiles] = useState(MockTemplateResourceFiles);
		const [path, setPath] = useState("main.tf");
		return (
			<>
				<button onClick={() => setPath("modules/coder-network-policy/main.tf")}>
					Open module
				</button>
				<button onClick={() => setPath("main.tf")}>Open root</button>
				<MonacoEditor
					path={path}
					value={getFileText(path, files)}
					onChange={(value) => {
						changed(path, value);
						setFiles((files) => updateFile(path, value, files));
					}}
				/>
			</>
		);
	}
	render(<EditorFiles />);
	await user.click(screen.getByRole("textbox", { name: "Editor content" }));
	await user.keyboard("{Control>}{End}{/Control}# draft");
	const root = monaco.editor.getModels()[0];
	if (!root) throw new Error("Root model was not created");
	const staleChange = callbacks.get(root.uri.toString());
	if (!staleChange)
		throw new Error("Root change subscription was not installed");
	await user.click(screen.getByRole("button", { name: "Open module" }));
	await user.click(screen.getByRole("textbox", { name: "Editor content" }));
	await user.keyboard("{Control>}{End}{/Control}# module");
	expect(changed).toHaveBeenLastCalledWith(
		"modules/coder-network-policy/main.tf",
		`${getFileText("modules/coder-network-policy/main.tf", MockTemplateResourceFiles)}# module`,
	);
	changed.mockClear();
	act(() => staleChange("wrong file contents"));
	expect(changed).not.toHaveBeenCalled();
	// Flush the model switch and its content subscription before imperative undo.
	await act(async () => {
		await user.click(screen.getByRole("button", { name: "Open root" }));
	});
	await act(async () => {
		await root.undo();
	});
	await waitFor(() =>
		expect(changed).toHaveBeenCalledWith(
			"main.tf",
			`${getFileText("main.tf", MockTemplateResourceFiles)}# draf`,
		),
	);
	expect(root.getValue()).not.toContain("# module");
});

it("updates a selected path when its revision changes without echoing prop updates as edits", async () => {
	const user = userEvent.setup();
	const changed = vi.fn();
	const view = render(
		<MonacoEditor path="main.tf" value="revision one" onChange={changed} />,
	);
	view.rerender(
		<MonacoEditor path="main.tf" value="revision two" onChange={changed} />,
	);
	expect(changed).not.toHaveBeenCalled();
	await user.click(screen.getByRole("textbox", { name: "Editor content" }));
	await user.keyboard("{End}!");
	expect(changed).toHaveBeenLastCalledWith("revision two!");
});
