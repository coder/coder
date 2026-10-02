import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { BannerConfig } from "#/api/typesGenerated";
import { render } from "#/testHelpers/renderHelpers";
import { AnnouncementBannerDialog } from "./AnnouncementBannerDialog";

const banner: BannerConfig = {
	enabled: true,
	message: "Hello from the banner",
	background_color: "#ffaff3",
};

describe("AnnouncementBannerDialog", () => {
	it("submits an edited message", async () => {
		const user = userEvent.setup();
		const onUpdate = vi.fn(async () => undefined);
		render(
			<AnnouncementBannerDialog
				banner={banner}
				onCancel={() => undefined}
				onUpdate={onUpdate}
			/>,
		);

		const message = screen.getByRole("textbox", { name: "Message" });
		await user.clear(message);
		await user.type(message, "Scheduled maintenance tonight.");
		await user.click(screen.getByRole("button", { name: "Update" }));

		expect(onUpdate).toHaveBeenCalledWith({
			message: "Scheduled maintenance tonight.",
			background_color: "#ffaff3",
		});
	});

	it("submits a preset background color", async () => {
		const user = userEvent.setup();
		const onUpdate = vi.fn(async () => undefined);
		render(
			<AnnouncementBannerDialog
				banner={banner}
				onCancel={() => undefined}
				onUpdate={onUpdate}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "#8b5cf6" }));
		await user.click(screen.getByRole("button", { name: "Update" }));

		expect(onUpdate).toHaveBeenCalledWith({
			message: "Hello from the banner",
			background_color: "#8b5cf6",
		});
	});

	it("submits a typed hex color", async () => {
		const user = userEvent.setup();
		const onUpdate = vi.fn(async () => undefined);
		render(
			<AnnouncementBannerDialog
				banner={banner}
				onCancel={() => undefined}
				onUpdate={onUpdate}
			/>,
		);

		const hex = screen.getByRole("textbox", { name: "Hex" });
		await user.clear(hex);
		await user.type(hex, "112233");
		await user.click(screen.getByRole("button", { name: "Update" }));

		expect(onUpdate).toHaveBeenCalledWith({
			message: "Hello from the banner",
			background_color: "#112233",
		});
	});

	it("keeps the current color when the hex value is incomplete", async () => {
		const user = userEvent.setup();
		const onUpdate = vi.fn(async () => undefined);
		render(
			<AnnouncementBannerDialog
				banner={banner}
				onCancel={() => undefined}
				onUpdate={onUpdate}
			/>,
		);

		const hex = screen.getByRole("textbox", { name: "Hex" });
		await user.clear(hex);
		await user.type(hex, "abc");
		await user.click(screen.getByRole("button", { name: "Update" }));

		expect(onUpdate).toHaveBeenCalledWith({
			message: "Hello from the banner",
			background_color: "#ffaff3",
		});
	});

	it("calls onCancel from the cancel button", async () => {
		const user = userEvent.setup();
		const onCancel = vi.fn();
		render(
			<AnnouncementBannerDialog
				banner={banner}
				onCancel={onCancel}
				onUpdate={async () => undefined}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Cancel" }));

		expect(onCancel).toHaveBeenCalledOnce();
	});
});
