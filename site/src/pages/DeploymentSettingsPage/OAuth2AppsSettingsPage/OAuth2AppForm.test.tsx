import { render, screen } from "@testing-library/react";
import { OAuth2AppForm } from "./OAuth2AppForm";

describe("OAuth2AppForm", () => {
	it("renders with empty fields when no props provided", () => {
		render(<OAuth2AppForm onSubmit={() => {}} isUpdating={false} />);

		const nameInput = screen.getByLabelText("Application name");
		const callbackInput = screen.getByLabelText("Callback URL");
		const iconInput = screen.getByLabelText("Application icon");

		expect(nameInput).toHaveValue("");
		expect(callbackInput).toHaveValue("");
		expect(iconInput).toHaveValue("");
	});

	it("renders with defaultValues when provided", () => {
		const defaultValues = {
			name: "Test App",
			callback_url: "https://example.com/callback",
			icon: "/icon/test.svg",
		};

		render(
			<OAuth2AppForm
				onSubmit={() => {}}
				isUpdating={false}
				defaultValues={defaultValues}
			/>,
		);

		const nameInput = screen.getByLabelText("Application name");
		const callbackInput = screen.getByLabelText("Callback URL");
		const iconInput = screen.getByLabelText("Application icon");

		expect(nameInput).toHaveValue("Test App");
		expect(callbackInput).toHaveValue("https://example.com/callback");
		expect(iconInput).toHaveValue("/icon/test.svg");
	});

	it("prefers app values over defaultValues when both provided", () => {
		const app = {
			id: "test-id",
			name: "App Name",
			callback_url: "https://app.com/callback",
			icon: "/icon/app.svg",
			endpoints: {
				authorization: "",
				token: "",
				device_authorization: "",
			},
		};

		const defaultValues = {
			name: "Default Name",
			callback_url: "https://default.com/callback",
			icon: "/icon/default.svg",
		};

		render(
			<OAuth2AppForm
				app={app}
				onSubmit={() => {}}
				isUpdating={false}
				defaultValues={defaultValues}
			/>,
		);

		const nameInput = screen.getByLabelText("Application name");
		const callbackInput = screen.getByLabelText("Callback URL");
		const iconInput = screen.getByLabelText("Application icon");

		// Should use app values, not default values
		expect(nameInput).toHaveValue("App Name");
		expect(callbackInput).toHaveValue("https://app.com/callback");
		expect(iconInput).toHaveValue("/icon/app.svg");
	});
});
