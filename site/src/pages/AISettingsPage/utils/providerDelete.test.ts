import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { cascadeDisableProviderModels } from "./providerDelete";

const model = (
	overrides: Partial<TypesGen.ChatModelConfig> &
		Pick<TypesGen.ChatModelConfig, "id" | "provider" | "model">,
): TypesGen.ChatModelConfig => ({
	id: overrides.id,
	provider: overrides.provider,
	ai_provider_id: overrides.ai_provider_id,
	model: overrides.model,
	display_name: overrides.display_name ?? overrides.model,
	enabled: overrides.enabled ?? true,
	is_default: overrides.is_default ?? false,
	context_limit: overrides.context_limit ?? 200000,
	compression_threshold: overrides.compression_threshold ?? 70,
	model_config: overrides.model_config,
	created_at: overrides.created_at ?? "2026-02-18T12:00:00.000Z",
	updated_at: overrides.updated_at ?? "2026-02-18T12:00:00.000Z",
});

describe("cascadeDisableProviderModels", () => {
	it("reassigns the default before disabling associated models", async () => {
		const associatedModels = [
			model({
				id: "model-associated-default",
				provider: "openai",
				model: "gpt-4o",
				is_default: true,
			}),
			model({
				id: "model-associated-secondary",
				provider: "openai",
				model: "gpt-4o-mini",
			}),
		];
		const allModels = [
			...associatedModels,
			model({
				id: "model-next-default",
				provider: "anthropic",
				model: "claude-sonnet-4",
			}),
		];
		const updateModelConfig = vi.fn(async () => undefined);

		await cascadeDisableProviderModels({
			associatedModels,
			allModels,
			updateModelConfig,
		});

		expect(updateModelConfig).toHaveBeenNthCalledWith(1, "model-next-default", {
			is_default: true,
		});
		expect(updateModelConfig).toHaveBeenNthCalledWith(
			2,
			"model-associated-default",
			{ enabled: false },
		);
		expect(updateModelConfig).toHaveBeenNthCalledWith(
			3,
			"model-associated-secondary",
			{ enabled: false },
		);
	});

	it("does not reassign the default when the deleted provider had no default", async () => {
		const updateModelConfig = vi.fn(async () => undefined);

		await cascadeDisableProviderModels({
			associatedModels: [
				model({
					id: "model-associated",
					provider: "openai",
					model: "gpt-4o",
				}),
			],
			allModels: [
				model({
					id: "model-associated",
					provider: "openai",
					model: "gpt-4o",
				}),
				model({
					id: "model-other-default",
					provider: "anthropic",
					model: "claude-sonnet-4",
					is_default: true,
				}),
			],
			updateModelConfig,
		});

		expect(updateModelConfig).toHaveBeenCalledTimes(1);
		expect(updateModelConfig).toHaveBeenCalledWith("model-associated", {
			enabled: false,
		});
	});

	it("rejects when default reassignment fails before disabling any models", async () => {
		const error = new Error("failed to reassign default");
		const updateModelConfig = vi.fn().mockRejectedValueOnce(error);

		await expect(
			cascadeDisableProviderModels({
				associatedModels: [
					model({
						id: "model-associated-default",
						provider: "openai",
						model: "gpt-4o",
						is_default: true,
					}),
					model({
						id: "model-associated-secondary",
						provider: "openai",
						model: "gpt-4o-mini",
					}),
				],
				allModels: [
					model({
						id: "model-next-default",
						provider: "anthropic",
						model: "claude-sonnet-4",
					}),
				],
				updateModelConfig,
			}),
		).rejects.toThrow(error);

		expect(updateModelConfig).toHaveBeenCalledTimes(1);
		expect(updateModelConfig).toHaveBeenNthCalledWith(1, "model-next-default", {
			is_default: true,
		});
		expect(updateModelConfig).not.toHaveBeenCalledWith(
			"model-associated-default",
			{ enabled: false },
		);
		expect(updateModelConfig).not.toHaveBeenCalledWith(
			"model-associated-secondary",
			{ enabled: false },
		);
	});

	it("rejects when a model disable fails after reassigning the default", async () => {
		const error = new Error("failed to disable model");
		const updateModelConfig = vi
			.fn()
			.mockResolvedValueOnce(undefined)
			.mockResolvedValueOnce(undefined)
			.mockRejectedValueOnce(error);

		await expect(
			cascadeDisableProviderModels({
				associatedModels: [
					model({
						id: "model-associated-default",
						provider: "openai",
						model: "gpt-4o",
						is_default: true,
					}),
					model({
						id: "model-associated-secondary",
						provider: "openai",
						model: "gpt-4o-mini",
					}),
				],
				allModels: [
					model({
						id: "model-next-default",
						provider: "anthropic",
						model: "claude-sonnet-4",
					}),
				],
				updateModelConfig,
			}),
		).rejects.toThrow(error);

		expect(updateModelConfig).toHaveBeenCalledTimes(3);
		expect(updateModelConfig).toHaveBeenNthCalledWith(1, "model-next-default", {
			is_default: true,
		});
	});

	it("does not reassign the default when no eligible fallback exists", async () => {
		const associatedModels = [
			model({
				id: "model-associated-default",
				provider: "openai",
				model: "gpt-4o",
				is_default: true,
			}),
			model({
				id: "model-associated-secondary",
				provider: "openai",
				model: "gpt-4o-mini",
			}),
		];
		const updateModelConfig = vi.fn(async () => undefined);

		await cascadeDisableProviderModels({
			associatedModels,
			allModels: associatedModels,
			updateModelConfig,
		});

		expect(updateModelConfig).toHaveBeenCalledTimes(2);
		expect(updateModelConfig).toHaveBeenNthCalledWith(
			1,
			"model-associated-default",
			{ enabled: false },
		);
		expect(updateModelConfig).toHaveBeenNthCalledWith(
			2,
			"model-associated-secondary",
			{ enabled: false },
		);
		expect(updateModelConfig).not.toHaveBeenCalledWith(expect.any(String), {
			is_default: true,
		});
	});
});
