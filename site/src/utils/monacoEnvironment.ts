import * as monaco from "monaco-editor";
import EditorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker";
import CssWorker from "monaco-editor/esm/vs/language/css/css.worker?worker";
import HtmlWorker from "monaco-editor/esm/vs/language/html/html.worker?worker";
import JsonWorker from "monaco-editor/esm/vs/language/json/json.worker?worker";
import TsWorker from "monaco-editor/esm/vs/language/typescript/ts.worker?worker";

/**
 * Import before creating an editor. Without getWorker, Monaco has no worker
 * for its editor service (word suggestions, links, diffs) and runs that code
 * on the main thread. Once getWorker exists, Monaco asks it for every worker,
 * so it serves the language workers too.
 */
self.MonacoEnvironment = {
	getWorker: (_workerId, label) => {
		switch (label) {
			case "json":
				return new JsonWorker();
			case "css":
			case "scss":
			case "less":
				return new CssWorker();
			case "html":
			case "handlebars":
			case "razor":
				return new HtmlWorker();
			case "typescript":
			case "javascript":
				return new TsWorker();
			default:
				return new EditorWorker();
		}
	},
};

// A standalone editor has no project to resolve imports and types from, so
// semantic checks report errors for every import. Syntax checks still run.
monaco.typescript.typescriptDefaults.setDiagnosticsOptions({
	...monaco.typescript.typescriptDefaults.getDiagnosticsOptions(),
	noSemanticValidation: true,
});
