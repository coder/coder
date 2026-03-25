import { renderHook } from "@testing-library/react";
import type {
	PreviewParameter,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import type { RefObject } from "react";
import { useSyncFormParameters } from "./useSyncFormParameters";

/**
 * Creates a minimal PreviewParameter with the given name and value.
 * Other required fields are filled with sensible defaults.
 */
function makeParam(
	name: string,
	value: string,
	valid: boolean,
): PreviewParameter {
	return {
		name,
		display_name: name,
		description: "",
		type: "string",
		form_type: "input",
		styling: {},
		mutable: true,
		default_value: { value: "", valid: true },
		icon: "",
		options: [],
		validations: [],
		required: false,
		order: 0,
		ephemeral: false,
		value: { value, valid },
		diagnostics: [],
	};
}

type Props = {
	parameters: readonly PreviewParameter[];
	formValues: readonly WorkspaceBuildParameter[];
	setFieldValue: (field: string, value: WorkspaceBuildParameter[]) => void;
	lastSentValues?: RefObject<Map<string, string>>;
};

function renderSyncHook(initialProps: Props) {
	return renderHook((props: Props) => useSyncFormParameters(props), {
		initialProps,
	});
}

describe("useSyncFormParameters", () => {
	it("updates form values when server returns new valid values", () => {
		const setFieldValue = vi.fn();
		const parameters = [
			makeParam("region", "us-east-1", true),
			makeParam("size", "large", true),
		];
		const formValues: WorkspaceBuildParameter[] = [
			{ name: "region", value: "us-west-2" },
			{ name: "size", value: "small" },
		];

		renderSyncHook({ parameters, formValues, setFieldValue });

		expect(setFieldValue).toHaveBeenCalledTimes(1);
		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "region", value: "us-east-1" },
			{ name: "size", value: "large" },
		]);
	});

	it("preserves existing form value when param.value.valid is false", () => {
		const setFieldValue = vi.fn();
		const parameters = [makeParam("region", "", false)];
		const formValues: WorkspaceBuildParameter[] = [
			{ name: "region", value: "us-west-2" },
		];

		renderSyncHook({ parameters, formValues, setFieldValue });

		// The form value should stay at "us-west-2" since the server
		// value is invalid. Because the form already holds "us-west-2",
		// the hook should not call setFieldValue (no change detected).
		expect(setFieldValue).not.toHaveBeenCalled();
	});

	it("uses empty string for invalid value when no existing form value exists", () => {
		const setFieldValue = vi.fn();
		const parameters = [makeParam("new_param", "", false)];
		// The form has no entry for "new_param" yet.
		const formValues: WorkspaceBuildParameter[] = [];

		renderSyncHook({ parameters, formValues, setFieldValue });

		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "new_param", value: "" },
		]);
	});

	it("preserves form value when server echoes back stale sent value", () => {
		const setFieldValue = vi.fn();

		// The user typed "hello w" and the server echoes "hello"
		// (the value we previously sent). The form has already
		// moved on to "hello w", so the hook should keep it.
		const lastSentValues: RefObject<Map<string, string>> = {
			current: new Map([["greeting", "hello"]]),
		};
		const parameters = [makeParam("greeting", "hello", true)];
		const formValues: WorkspaceBuildParameter[] = [
			{ name: "greeting", value: "hello w" },
		];

		renderSyncHook({
			parameters,
			formValues,
			setFieldValue,
			lastSentValues,
		});

		// The form already holds the desired value "hello w", so
		// setFieldValue should not be called (no change).
		expect(setFieldValue).not.toHaveBeenCalled();
	});

	it("applies server transformation even when form has diverged", () => {
		const setFieldValue = vi.fn();

		// The server returned "HELLO" (a transformation of the
		// sent value "hello"). Even though the form has "hello w",
		// the server value differs from what we sent, so it should
		// be applied.
		const lastSentValues: RefObject<Map<string, string>> = {
			current: new Map([["greeting", "hello"]]),
		};
		const parameters = [makeParam("greeting", "HELLO", true)];
		const formValues: WorkspaceBuildParameter[] = [
			{ name: "greeting", value: "hello w" },
		];

		renderSyncHook({
			parameters,
			formValues,
			setFieldValue,
			lastSentValues,
		});

		// The server transformed the value, so the hook applies it.
		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "greeting", value: "HELLO" },
		]);
	});

	it("applies server value when form matches lastSentValues (no user divergence)", () => {
		const setFieldValue = vi.fn();

		// The user has not typed anything new since we sent "hello",
		// so the server echo should be applied as-is.
		const lastSentValues: RefObject<Map<string, string>> = {
			current: new Map([["greeting", "hello"]]),
		};
		const parameters = [makeParam("greeting", "hello", true)];
		const formValues: WorkspaceBuildParameter[] = [
			{ name: "greeting", value: "hello" },
		];

		renderSyncHook({
			parameters,
			formValues,
			setFieldValue,
			lastSentValues,
		});

		// No change because form already matches the server value.
		expect(setFieldValue).not.toHaveBeenCalled();
	});

	it("behaves identically to basic sync when lastSentValues is not provided", () => {
		const setFieldValue = vi.fn();
		const parameters = [makeParam("region", "us-east-1", true)];
		const formValues: WorkspaceBuildParameter[] = [
			{ name: "region", value: "us-west-2" },
		];

		// No lastSentValues provided — backward-compatible path.
		renderSyncHook({ parameters, formValues, setFieldValue });

		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "region", value: "us-east-1" },
		]);
	});

	it("does not call setFieldValue when form values already match parameters", () => {
		const setFieldValue = vi.fn();
		const parameters = [makeParam("region", "us-east-1", true)];
		const formValues: WorkspaceBuildParameter[] = [
			{ name: "region", value: "us-east-1" },
		];

		renderSyncHook({ parameters, formValues, setFieldValue });

		expect(setFieldValue).not.toHaveBeenCalled();
	});

	it("re-syncs when parameters change across renders", () => {
		const setFieldValue = vi.fn();
		const formValues: WorkspaceBuildParameter[] = [
			{ name: "region", value: "us-west-2" },
		];
		const initialParams = [makeParam("region", "us-west-2", true)];

		const { rerender } = renderSyncHook({
			parameters: initialParams,
			formValues,
			setFieldValue,
		});

		// Initial render: no change because form matches.
		expect(setFieldValue).not.toHaveBeenCalled();

		// Server pushes a new parameter value.
		const updatedParams = [makeParam("region", "eu-west-1", true)];
		rerender({
			parameters: updatedParams,
			formValues,
			setFieldValue,
		});

		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "region", value: "eu-west-1" },
		]);
	});

	it("handles multiple parameters with mixed stale and fresh responses", () => {
		const setFieldValue = vi.fn();

		const lastSentValues: RefObject<Map<string, string>> = {
			current: new Map([
				["greeting", "hello"],
				["farewell", "bye"],
			]),
		};

		const parameters = [
			// Stale: server echoes "hello" but form has "hello w".
			makeParam("greeting", "hello", true),
			// Fresh transformation: server returns "GOODBYE" (not "bye").
			makeParam("farewell", "GOODBYE", true),
		];

		const formValues: WorkspaceBuildParameter[] = [
			{ name: "greeting", value: "hello w" },
			{ name: "farewell", value: "bye" },
		];

		renderSyncHook({
			parameters,
			formValues,
			setFieldValue,
			lastSentValues,
		});

		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			// Stale response: form value preserved.
			{ name: "greeting", value: "hello w" },
			// Server transformation: applied.
			{ name: "farewell", value: "GOODBYE" },
		]);
	});
});
