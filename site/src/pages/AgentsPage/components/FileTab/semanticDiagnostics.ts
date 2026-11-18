import type * as monaco from "monaco-editor";

type TypeScriptDefaults = typeof monaco.typescript.typescriptDefaults;
type DiagnosticsDefaults = Pick<
	TypeScriptDefaults,
	"getDiagnosticsOptions" | "setDiagnosticsOptions"
>;
type DiagnosticsOptions = ReturnType<
	TypeScriptDefaults["getDiagnosticsOptions"]
>;

/**
 * Returns a function that turns off TypeScript semantic checks until the
 * function it returns is called. A standalone editor has no project to
 * resolve imports and types from, so semantic checks report errors for every
 * import; syntax checks still run. The options are global to Monaco, so the
 * options in effect before the first caller come back once the last caller
 * releases, and other editors in the app, such as the template editor, keep
 * their semantic checks.
 */
export const createSemanticDiagnosticsSuppressor = (
	defaults: DiagnosticsDefaults,
) => {
	let holders = 0;
	let restored: DiagnosticsOptions | undefined;
	return () => {
		if (holders === 0) {
			restored = defaults.getDiagnosticsOptions();
			defaults.setDiagnosticsOptions({
				...restored,
				noSemanticValidation: true,
			});
		}
		holders++;
		let isReleased = false;
		return () => {
			if (isReleased) {
				return;
			}
			isReleased = true;
			holders--;
			if (holders === 0 && restored) {
				defaults.setDiagnosticsOptions(restored);
				restored = undefined;
			}
		};
	};
};
