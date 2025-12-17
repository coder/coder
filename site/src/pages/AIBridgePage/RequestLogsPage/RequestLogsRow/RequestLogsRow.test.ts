import { tokenUsageMetadataMerge } from "./RequestLogsRow";

describe("tokenUsageMetadataMerge", () => {
	it("returns null when inputs are null or empty", () => {
		const result = tokenUsageMetadataMerge(null, {}, null);

		expect(result).toBeNull();
	});

	it("returns data when there are no shared keys across metadata", () => {
		const metadataA = { input_tokens: 5 };
		const metadataB = { output_tokens: 2 };

		const result = tokenUsageMetadataMerge(metadataA, metadataB);

		expect(result).toEqual([metadataA, metadataB]);
	});

	it("sums numeric values for common keys and keeps non-common keys", () => {
		const metadataA = {
			input_tokens: 5,
			model: "gpt-4",
		};
		const metadataB = {
			input_tokens: 3,
			output_tokens: 2,
		};

		const result = tokenUsageMetadataMerge(metadataA, metadataB);

		expect(result).toEqual({
			input_tokens: 8,
			model: "gpt-4",
			output_tokens: 2,
		});
	});

	it("preserves identical non-numeric values for common keys", () => {
		const metadataA = {
			note: "sync",
			status: "ok",
		};
		const metadataB = {
			note: "sync",
			status: "ok",
		};

		const result = tokenUsageMetadataMerge(metadataA, metadataB);

		expect(result).toEqual({
			note: "sync",
			status: "ok",
		});
	});

	it("returns the original metadata array when a conflict cannot be resolved", () => {
		const metadataA = {
			input_tokens: 1,
			label: "a",
		};
		const metadataB = {
			input_tokens: 3,
			label: "b",
		};

		const result = tokenUsageMetadataMerge(metadataA, metadataB);

		expect(result).toEqual([metadataA, metadataB]);
	});
});
