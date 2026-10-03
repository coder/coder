import { describe, expect, it } from "vitest";
import type { ChatModel } from "#/api/typesGenerated";
import { MockChatModel } from "#/testHelpers/chatModels";
import { resolveSpawnModelDisplay } from "./spawnModelDisplay";

const config = (overrides: Partial<ChatModel>): ChatModel => ({
	...MockChatModel,
	...overrides,
});

const sonnet = config({
	id: "cfg-sonnet",
	model: "claude-sonnet-4-6",
	display_name: "Claude Sonnet 4.6",
	model_config: {
		reasoning_effort: { default: "medium", max: "high" },
	},
});

const noEffortModel = config({
	id: "cfg-plain",
	model: "gpt-4.1",
	display_name: "GPT-4.1",
});

describe("resolveSpawnModelDisplay", () => {
	it("returns nothing without explicit args", () => {
		expect(
			resolveSpawnModelDisplay({ models: [sonnet], modelId: undefined }),
		).toEqual({});
	});

	it("resolves model and explicit effort", () => {
		expect(
			resolveSpawnModelDisplay({
				models: [sonnet],
				modelId: "cfg-sonnet",
				reasoningEffort: "low",
			}),
		).toEqual({ modelLabel: "Claude Sonnet 4.6", effortLabel: "low" });
	});

	it("falls back to the config default effort when not requested", () => {
		expect(
			resolveSpawnModelDisplay({
				models: [sonnet],
				modelId: "cfg-sonnet",
			}),
		).toEqual({ modelLabel: "Claude Sonnet 4.6", effortLabel: "medium" });
	});

	it("clamps a requested effort above the config max", () => {
		expect(
			resolveSpawnModelDisplay({
				models: [sonnet],
				modelId: "cfg-sonnet",
				reasoningEffort: "max",
			}),
		).toEqual({ modelLabel: "Claude Sonnet 4.6", effortLabel: "high" });
	});

	it("omits effort for models without effort settings", () => {
		expect(
			resolveSpawnModelDisplay({
				models: [noEffortModel],
				modelId: "cfg-plain",
				reasoningEffort: "high",
			}),
		).toEqual({ modelLabel: "GPT-4.1" });
	});

	it("shows nothing when the config is unknown", () => {
		expect(
			resolveSpawnModelDisplay({
				models: [sonnet],
				modelId: "deleted-config",
				reasoningEffort: "xhigh",
			}),
		).toEqual({});
	});

	it("shows nothing for effort-only spawns", () => {
		expect(
			resolveSpawnModelDisplay({
				models: [sonnet],
				reasoningEffort: "high",
			}),
		).toEqual({});
	});

	it("omits an effective effort of none", () => {
		expect(
			resolveSpawnModelDisplay({
				models: [sonnet],
				modelId: "cfg-sonnet",
				reasoningEffort: "none",
			}),
		).toEqual({ modelLabel: "Claude Sonnet 4.6" });
		expect(
			resolveSpawnModelDisplay({
				models: [sonnet],
				reasoningEffort: "none",
			}),
		).toEqual({});
	});

	it("ignores values outside the global scale", () => {
		expect(
			resolveSpawnModelDisplay({
				models: [sonnet],
				modelId: "cfg-sonnet",
				reasoningEffort: "ultra",
			}),
		).toEqual({ modelLabel: "Claude Sonnet 4.6", effortLabel: "medium" });
		expect(
			resolveSpawnModelDisplay({ models: [sonnet], reasoningEffort: "ultra" }),
		).toEqual({});
	});

	it("falls back to the model id when display name is blank", () => {
		const blankName = config({
			id: "cfg-blank",
			model: "o4-mini",
			display_name: "  ",
		});
		expect(
			resolveSpawnModelDisplay({
				models: [blankName],
				modelId: "cfg-blank",
			}),
		).toEqual({ modelLabel: "o4-mini" });
	});

	it("returns nothing while models are still loading", () => {
		expect(
			resolveSpawnModelDisplay({
				models: undefined,
				modelId: "cfg-sonnet",
			}),
		).toEqual({});
	});
});
