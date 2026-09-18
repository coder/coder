import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PersonalMemorySettings } from "./PersonalMemorySettings";

describe("PersonalMemorySettings", () => {
	it("renders nothing without settings", () => {
		const { container } = render(
			<PersonalMemorySettings
				settings={undefined}
				onSaveSettings={vi.fn()}
				isSavingSettings={false}
				isSaveSettingsError={false}
			/>,
		);
		expect(container).toBeEmptyDOMElement();
	});

	it("saves the toggled value", async () => {
		const user = userEvent.setup();
		const onSaveSettings = vi.fn();
		render(
			<PersonalMemorySettings
				settings={{ enabled: true }}
				onSaveSettings={onSaveSettings}
				isSavingSettings={false}
				isSaveSettingsError={false}
			/>,
		);

		await user.click(
			screen.getByRole("switch", { name: "Save and use personal memory" }),
		);

		expect(onSaveSettings).toHaveBeenCalledWith({ enabled: false });
	});
});
