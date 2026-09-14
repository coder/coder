import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { JsonPrettyPrinter } from "./JsonPrettyPrinter";

const PreviewBlock: FC<{ input: string; tool?: string }> = ({
	input,
	tool,
}) => (
	<pre className="p-4 bg-surface-secondary rounded text-xs overflow-x-auto">
		{tool} <JsonPrettyPrinter input={input} />
	</pre>
);

const meta: Meta<typeof PreviewBlock> = {
	title: "pages/AIBridgePage/JsonPrettyPrinter",
	component: PreviewBlock,
};

export default meta;
type Story = StoryObj<typeof PreviewBlock>;

export const FlatObject: Story = {
	args: {
		input: JSON.stringify({
			name: "claude-opus-4-5",
			provider: "anthropic",
			max_tokens: 4096,
			streaming: true,
		}),
	},
};

export const NestedObject: Story = {
	args: {
		input: JSON.stringify({
			model: "claude-opus-4-5",
			usage: {
				input_tokens: 1234,
				output_tokens: 567,
				cache_read_input_tokens: 800,
				cache_creation_input_tokens: 200,
			},
			stop_reason: "end_turn",
			stop_sequence: null,
		}),
	},
};

export const ArrayOfObjects: Story = {
	args: {
		input: JSON.stringify([
			{ role: "user", content: "Hello" },
			{ role: "assistant", content: "Hi there!" },
			{ role: "user", content: "How are you?" },
		]),
	},
};

export const MixedTypes: Story = {
	args: {
		input: JSON.stringify({
			string_value: "hello world",
			number_value: 42,
			float_value: 3.14,
			bool_true: true,
			bool_false: false,
			null_value: null,
			array_value: [1, "two", true, null],
		}),
	},
};

export const EmptyObject: Story = {
	args: {
		input: JSON.stringify({}),
	},
};

export const EmptyArray: Story = {
	args: {
		input: JSON.stringify([]),
	},
};

export const InvalidJSON: Story = {
	args: {
		input: "not valid json {",
	},
};

export const DeepNesting: Story = {
	args: {
		input: JSON.stringify({
			level1: {
				level2: {
					level3: {
						value: "deep",
						count: 3,
					},
				},
			},
		}),
	},
};

export const WithToolName: Story = {
	args: {
		input: JSON.stringify({
			pattern: "UTC_OFFSET|timeZoneName|DateTimeFormat",
			path: "/home/coder/coder/site/src/utils/time.ts",
			output_mode: "content",
		}),
		tool: "Grep",
	},
};
