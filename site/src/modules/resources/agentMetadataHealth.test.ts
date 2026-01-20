import type { WorkspaceAgentMetadata } from "api/typesGenerated";
import {
	ZERO_TIME_ISO,
	isInvalidAgentMetadataSample,
	isValidAgentMetadataSample,
} from "./agentMetadataHealth";

const makeItem = (overrides: Partial<WorkspaceAgentMetadata>): WorkspaceAgentMetadata => {
	return {
		description: {
			display_name: "CPU Usage",
			key: "cpu_usage",
			script: "coder stat cpu",
			interval: 30,
			timeout: 1,
		},
		result: {
			collected_at: new Date().toISOString(),
			age: 0,
			value: "0.1/4 cores (1%)\n",
			error: "",
		},
		...overrides,
	} as const satisfies WorkspaceAgentMetadata;
};

describe("agentMetadataHealth", () => {
	it("treats zero-time + empty values as invalid", () => {
		const sample: WorkspaceAgentMetadata[] = [
			makeItem({
				result: {
					collected_at: ZERO_TIME_ISO,
					age: 9223372036,
					value: "",
					error: "",
				},
			}),
		];

		expect(isInvalidAgentMetadataSample(sample)).toBe(true);
		expect(isValidAgentMetadataSample(sample)).toBe(false);
	});

	it("treats real collected_at + non-empty value as valid", () => {
		const sample: WorkspaceAgentMetadata[] = [makeItem({})];

		expect(isValidAgentMetadataSample(sample)).toBe(true);
		expect(isInvalidAgentMetadataSample(sample)).toBe(false);
	});
});

