import { renderHook, waitFor } from "@testing-library/react";
import {
	MockDropdownParameter,
	MockPreviewParameter1,
	MockPreviewParameter2,
} from "#/testHelpers/entities";
import type { AutofillBuildParameter } from "#/utils/richParameters";
import { useSyncFormParameters } from "./useSyncFormParameters";

describe("useSyncFormParameters", () => {
	it("applies autofill when a parameter is introduced", async () => {
		const formValues = [
			{
				name: MockPreviewParameter1.name,
				value: MockPreviewParameter1.value.value,
			},
		];
		const autofillParameters: AutofillBuildParameter[] = [
			{
				name: MockPreviewParameter2.name,
				value: "3",
				source: "url",
			},
		];
		const setFieldValue = vi.fn();
		const setFieldTouched = vi.fn();
		const { rerender } = renderHook(
			({ parameters }) =>
				useSyncFormParameters({
					parameters,
					autofillParameters,
					formValues,
					touched: {},
					setFieldValue,
					setFieldTouched,
				}),
			{ initialProps: { parameters: [MockPreviewParameter1] } },
		);

		expect(setFieldValue).not.toHaveBeenCalled();

		rerender({
			parameters: [MockPreviewParameter1, MockPreviewParameter2],
		});

		await waitFor(() => {
			expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
				...formValues,
				{ name: MockPreviewParameter2.name, value: "3" },
			]);
		});
		expect(setFieldTouched).toHaveBeenCalledWith(
			MockPreviewParameter2.name,
			true,
			false,
		);
	});

	it("ignores invalid autofill when a parameter is introduced", async () => {
		const formValues = [
			{
				name: MockPreviewParameter1.name,
				value: MockPreviewParameter1.value.value,
			},
		];
		const setFieldValue = vi.fn();
		const setFieldTouched = vi.fn();
		const { rerender } = renderHook(
			({ parameters }) =>
				useSyncFormParameters({
					parameters,
					autofillParameters: [
						{
							name: MockDropdownParameter.name,
							value: "not-an-option",
							source: "url",
						},
					],
					formValues,
					touched: {},
					setFieldValue,
					setFieldTouched,
				}),
			{ initialProps: { parameters: [MockPreviewParameter1] } },
		);

		rerender({
			parameters: [MockPreviewParameter1, MockDropdownParameter],
		});

		await waitFor(() => {
			expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
				...formValues,
				{
					name: MockDropdownParameter.name,
					value: MockDropdownParameter.value.value,
				},
			]);
		});
		expect(setFieldTouched).not.toHaveBeenCalled();
	});

	it("ignores autofill for an ephemeral parameter", async () => {
		const formValues = [
			{
				name: MockPreviewParameter1.name,
				value: MockPreviewParameter1.value.value,
			},
		];
		const ephemeralParameter = {
			...MockPreviewParameter2,
			ephemeral: true,
		};
		const setFieldValue = vi.fn();
		const setFieldTouched = vi.fn();
		const { rerender } = renderHook(
			({ parameters }) =>
				useSyncFormParameters({
					parameters,
					autofillParameters: [
						{
							name: ephemeralParameter.name,
							value: "3",
							source: "url",
						},
					],
					formValues,
					touched: {},
					setFieldValue,
					setFieldTouched,
				}),
			{ initialProps: { parameters: [MockPreviewParameter1] } },
		);

		rerender({
			parameters: [MockPreviewParameter1, ephemeralParameter],
		});

		await waitFor(() => {
			expect(setFieldValue).toHaveBeenCalledWith("rich_parameter_values", [
				...formValues,
				{
					name: ephemeralParameter.name,
					value: ephemeralParameter.value.value,
				},
			]);
		});
		expect(setFieldTouched).not.toHaveBeenCalled();
	});
});
