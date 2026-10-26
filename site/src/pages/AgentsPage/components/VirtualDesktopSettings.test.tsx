import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockAIProviderAnthropic,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { VirtualDesktopSettings } from "./VirtualDesktopSettings";

const anthropicProvider: TypesGen.AIProvider = {
	...MockAIProviderAnthropic,
	enabled: true,
};
const openAIProvider: TypesGen.AIProvider = {
	...MockAIProviderOpenAI,
	display_name: "Team OpenAI",
	enabled: true,
};

describe("VirtualDesktopSettings", () => {
	it("saves the chosen AI provider with its type", async () => {
		const user = userEvent.setup();
		const onSave = vi.fn();
		render(
			<VirtualDesktopSettings
				computerUseProviderData={{
					provider: "anthropic",
					ai_provider_id: anthropicProvider.id,
				}}
				isLoadingComputerUseProvider={false}
				aiProviders={[anthropicProvider, openAIProvider]}
				isLoadingAIProviders={false}
				aiProvidersError={null}
				onSaveComputerUseProvider={onSave}
				isSavingComputerUseProvider={false}
				computerUseProviderSaveError={null}
			/>,
		);

		await user.click(
			screen.getByRole("combobox", { name: "Computer use provider" }),
		);
		await user.click(
			await screen.findByRole("option", { name: /Team OpenAI/ }),
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSave).toHaveBeenCalledWith(
			{ provider: "openai", ai_provider_id: openAIProvider.id },
			expect.anything(),
		);
	});
});
