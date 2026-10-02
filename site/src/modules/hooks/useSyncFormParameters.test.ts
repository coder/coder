import { renderHook } from "@testing-library/react";
import type { PreviewParameter } from "#/api/typesGenerated";
import type { AutofillBuildParameter } from "#/utils/richParameters";
import {
	type UseSyncFormParametersProps,
	useSyncFormParameters,
} from "./useSyncFormParameters";

const createParam = (
	overrides: Partial<PreviewParameter> = {},
): PreviewParameter => ({
	name: "test_param",
	display_name: "Test Parameter",
	description: "",
	type: "string",
	form_type: "input",
	styling: {},
	mutable: true,
	default_value: { value: "", valid: true },
	icon: "",
	options: [],
	validations: [],
	value: { value: "", valid: true },
	diagnostics: [],
	required: false,
	order: 0,
	ephemeral: false,
	...overrides,
});

const touched = (
	obj: Record<string, boolean>,
): UseSyncFormParametersProps["touched"] =>
	obj as UseSyncFormParametersProps["touched"];

const createAutofill = (
	overrides: Partial<AutofillBuildParameter> = {},
): AutofillBuildParameter => ({
	source: "url",
	name: "test_param",
	value: "",
	...overrides,
});

describe(useSyncFormParameters.name, () => {
	it("applies the URL autofill value to a conditional parameter the first time it appears", () => {
		// app_1_name is gated by an app_count parameter and only appears in
		// `parameters` once that gate resolves. Its URL-provided autofill
		// value was never appliable at form init (it didn't exist yet), so
		// this is the only chance to apply it.
		const conditionalParam = createParam({ name: "app_1_name" });
		const autofill = createAutofill({ name: "app_1_name", value: "my-app" });

		const setFieldValue = vi.fn();
		const setFieldTouched = vi.fn();

		renderHook(() =>
			useSyncFormParameters({
				parameters: [conditionalParam],
				formValues: [],
				touched: {},
				autofillByName: { [autofill.name]: autofill },
				setFieldValue,
				setFieldTouched,
			}),
		);

		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "app_1_name", value: "my-app" },
		]);
		expect(setFieldTouched).toHaveBeenCalledWith("app_1_name", true);
	});

	it("does not override a value the user already touched, even if it has an autofill entry", () => {
		const conditionalParam = createParam({ name: "app_1_name" });
		const autofill = createAutofill({ name: "app_1_name", value: "my-app" });

		const setFieldValue = vi.fn();
		const setFieldTouched = vi.fn();

		renderHook(() =>
			useSyncFormParameters({
				parameters: [conditionalParam],
				formValues: [{ name: "app_1_name", value: "user-typed-value" }],
				touched: touched({ app_1_name: true }),
				autofillByName: { [autofill.name]: autofill },
				setFieldValue,
				setFieldTouched,
			}),
		);

		expect(setFieldValue).not.toHaveBeenCalled();
		expect(setFieldTouched).not.toHaveBeenCalled();
	});

	it("falls back to the parameter's own value when there is no autofill entry", () => {
		const param = createParam({
			name: "plain_param",
			value: { value: "default", valid: true },
		});

		const setFieldValue = vi.fn();
		const setFieldTouched = vi.fn();

		renderHook(() =>
			useSyncFormParameters({
				parameters: [param],
				formValues: [],
				touched: {},
				autofillByName: {},
				setFieldValue,
				setFieldTouched,
			}),
		);

		expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
			{ name: "plain_param", value: "default" },
		]);
		expect(setFieldTouched).not.toHaveBeenCalled();
	});
});
