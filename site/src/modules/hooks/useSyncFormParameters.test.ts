import { renderHook } from "@testing-library/react";
import type { PreviewParameter } from "api/typesGenerated";
import { useSyncFormParameters } from "./useSyncFormParameters";

function makeParam(
	name: string,
	value: string,
	valid = true,
): PreviewParameter {
	return {
		name,
		display_name: name,
		description: "",
		type: "string",
		form_type: "input",
		styling: {},
		mutable: true,
		default_value: { value: "", valid: false },
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

describe("useSyncFormParameters", () => {
	it("syncs untouched fields from server", () => {
		const setFieldValue = vi.fn();
		const parameters = [makeParam("region", "us-east")];
		const formValues = [{ name: "region", value: "us-west" }];

		renderHook(() =>
			useSyncFormParameters({
				parameters,
				formValues,
				touched: {},
				setFieldValue,
			}),
		);

		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "region", value: "us-east" },
		]);
	});

	it("preserves touched fields when form value differs from server", () => {
		const setFieldValue = vi.fn();
		// Server still has old value "false", but user toggled to "true".
		const parameters = [makeParam("use_gpu", "false")];
		const formValues = [{ name: "use_gpu", value: "true" }];

		renderHook(() =>
			useSyncFormParameters({
				parameters,
				formValues,
				touched: { use_gpu: true },
				setFieldValue,
			}),
		);

		// The hook should not overwrite the user's value.
		expect(setFieldValue).not.toHaveBeenCalled();
	});

	it("syncs touched fields when server matches form value", () => {
		const setFieldValue = vi.fn();
		// Server has echoed back the user's value.
		const parameters = [
			makeParam("use_gpu", "true"),
			makeParam("region", "eu-west"),
		];
		const formValues = [
			{ name: "use_gpu", value: "true" },
			{ name: "region", value: "us-east" },
		];

		renderHook(() =>
			useSyncFormParameters({
				parameters,
				formValues,
				touched: { use_gpu: true },
				setFieldValue,
			}),
		);

		// region is untouched and differs, so it should sync.
		// use_gpu matches server, so it syncs normally too.
		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "use_gpu", value: "true" },
			{ name: "region", value: "eu-west" },
		]);
	});

	it("adds new parameters from server", () => {
		const setFieldValue = vi.fn();
		const parameters = [
			makeParam("region", "us-east"),
			makeParam("instance_type", "t3.large"),
		];
		const formValues = [{ name: "region", value: "us-east" }];

		renderHook(() =>
			useSyncFormParameters({
				parameters,
				formValues,
				touched: {},
				setFieldValue,
			}),
		);

		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "region", value: "us-east" },
			{ name: "instance_type", value: "t3.large" },
		]);
	});

	it("removes parameters no longer in server response", () => {
		const setFieldValue = vi.fn();
		// Server only has "region", but form still has "old_param".
		const parameters = [makeParam("region", "us-east")];
		const formValues = [
			{ name: "region", value: "us-east" },
			{ name: "old_param", value: "stale" },
		];

		renderHook(() =>
			useSyncFormParameters({
				parameters,
				formValues,
				touched: {},
				setFieldValue,
			}),
		);

		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "region", value: "us-east" },
		]);
	});

	it("does not call setFieldValue when nothing changed", () => {
		const setFieldValue = vi.fn();
		const parameters = [makeParam("region", "us-east")];
		const formValues = [{ name: "region", value: "us-east" }];

		renderHook(() =>
			useSyncFormParameters({
				parameters,
				formValues,
				touched: {},
				setFieldValue,
			}),
		);

		expect(setFieldValue).not.toHaveBeenCalled();
	});
});
