import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockAIProviderAnthropic,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import { render, renderComponent } from "#/testHelpers/renderHelpers";
import { VirtualDesktopSettings } from "./VirtualDesktopSettings";

const mockEnabledAnthropicProvider: TypesGen.AIProvider = {
	...MockAIProviderAnthropic,
	enabled: true,
};
const mockBackupAnthropicProvider: TypesGen.AIProvider = {
	...mockEnabledAnthropicProvider,
	id: "0d7c1f7e-35b8-4c5e-9d0f-6a1c2e3b4d5f",
	name: "anthropic-backup",
};
const mockTeamOpenAIProvider: TypesGen.AIProvider = {
	...MockAIProviderOpenAI,
	display_name: "Team OpenAI",
	enabled: true,
};

const defaultProps: React.ComponentProps<typeof VirtualDesktopSettings> = {
	computerUseProviderData: {
		provider: "anthropic",
		ai_provider_id: mockEnabledAnthropicProvider.id,
	},
	isLoadingComputerUseProvider: false,
	aiProviders: [mockEnabledAnthropicProvider, mockTeamOpenAIProvider],
	isLoadingAIProviders: false,
	aiProvidersError: null,
	onSaveComputerUseProvider: () => {},
	isSavingComputerUseProvider: false,
	computerUseProviderSaveError: null,
};

const getProviderPicker = () =>
	screen.getByRole("combobox", { name: "Computer use provider" });

describe("VirtualDesktopSettings", () => {
	it("saves the chosen AI provider with its type", async () => {
		const user = userEvent.setup();
		const onSave = vi.fn();
		render(
			<VirtualDesktopSettings
				{...defaultProps}
				onSaveComputerUseProvider={onSave}
			/>,
		);

		await user.click(getProviderPicker());
		await user.click(
			await screen.findByRole("option", { name: /Team OpenAI/ }),
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSave).toHaveBeenCalledWith(
			{ provider: "openai", ai_provider_id: mockTeamOpenAIProvider.id },
			expect.anything(),
		);
	});

	it("saves the stored provider to repair its type after a late load", async () => {
		const user = userEvent.setup();
		const onSave = vi.fn();
		const mismatchedProps: React.ComponentProps<typeof VirtualDesktopSettings> =
			{
				...defaultProps,
				computerUseProviderData: {
					provider: "openai",
					ai_provider_id: mockEnabledAnthropicProvider.id,
				},
				onSaveComputerUseProvider: onSave,
			};
		// Setting the stored value before its options load makes Radix emit
		// an empty change, which must not clear the stored selection.
		const { rerender } = renderComponent(
			<VirtualDesktopSettings
				{...mismatchedProps}
				computerUseProviderData={undefined}
				isLoadingComputerUseProvider
				aiProviders={undefined}
				isLoadingAIProviders
			/>,
		);
		rerender(
			<VirtualDesktopSettings
				{...mismatchedProps}
				aiProviders={undefined}
				isLoadingAIProviders
			/>,
		);
		rerender(<VirtualDesktopSettings {...mismatchedProps} />);

		await user.click(await screen.findByRole("button", { name: "Save" }));

		expect(onSave).toHaveBeenCalledWith(
			{
				provider: "anthropic",
				ai_provider_id: mockEnabledAnthropicProvider.id,
			},
			expect.anything(),
		);
	});

	it("tells providers with the same display name apart", async () => {
		const user = userEvent.setup();
		const onSave = vi.fn();
		render(
			<VirtualDesktopSettings
				{...defaultProps}
				aiProviders={[
					mockEnabledAnthropicProvider,
					mockBackupAnthropicProvider,
				]}
				onSaveComputerUseProvider={onSave}
			/>,
		);

		await user.click(getProviderPicker());
		await user.click(
			await screen.findByRole("option", { name: /anthropic-backup/ }),
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSave).toHaveBeenCalledWith(
			{ provider: "anthropic", ai_provider_id: mockBackupAnthropicProvider.id },
			expect.anything(),
		);
	});

	it("sends one request per Save until the previous one settles", async () => {
		const user = userEvent.setup();
		const onSave = vi.fn();
		render(
			<VirtualDesktopSettings
				{...defaultProps}
				onSaveComputerUseProvider={onSave}
			/>,
		);

		await user.click(getProviderPicker());
		await user.click(
			await screen.findByRole("option", { name: /Team OpenAI/ }),
		);
		const saveButton = screen.getByRole("button", { name: "Save" });
		await user.click(saveButton);
		await user.click(saveButton);
		await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));

		act(() => onSave.mock.calls[0][1].onSettled());
		await user.click(saveButton);

		await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2));
	});
});
