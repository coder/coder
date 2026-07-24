import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import type { DynamicParametersResponse } from "#/api/typesGenerated";
import { MockPreviewParameter, MockTemplate } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { mockDynamicParameterWebSocket } from "#/testHelpers/websockets";
import { TemplateLayout } from "../TemplateLayout";
import TemplateEmbedPage from "./TemplateEmbedPage";

// Renders TemplateEmbedPage inside the real TemplateLayout so that
// useTemplateLayoutContext and useAuthenticated are both satisfied through
// the normal provider tree + MSW handlers.
function renderEmbedPage() {
	return renderWithAuth(<TemplateLayout />, {
		path: "/templates/:template",
		route: `/templates/${MockTemplate.name}/embed`,
		children: [{ path: "embed", element: <TemplateEmbedPage /> }],
	});
}

function getSearchParams(url: string): URLSearchParams {
	const startOf = url.indexOf("?");
	if (startOf < 0) {
		return new URLSearchParams();
	}
	return new URLSearchParams(url.slice(startOf));
}

const paramRegion = {
	...MockPreviewParameter,
	name: "region",
	display_name: "Region",
	form_type: "input" as const,
	value: { value: "us-east-1", valid: true },
	default_value: { value: "us-east-1", valid: true },
	order: 0,
};
const paramCpu = {
	...MockPreviewParameter,
	name: "cpu",
	display_name: "CPU",
	form_type: "input" as const,
	value: { value: "4", valid: true },
	default_value: { value: "4", valid: true },
	order: 1,
};

const writeTextMock = vi.fn().mockResolvedValue(undefined);

describe("TemplateEmbedPage", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.stubGlobal("navigator", {
			...navigator,
			clipboard: { writeText: writeTextMock },
		});
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	it("populates parameters", async () => {
		mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: 0,
						parameters: [paramRegion, paramCpu],
						diagnostics: [],
					}),
				}),
			);
		});

		renderEmbedPage();

		await waitFor(() => {
			expect(screen.getByDisplayValue("us-east-1")).toBeInTheDocument();
		});

		const cpuInput = screen.getByDisplayValue("4");
		expect(cpuInput).toBeInTheDocument();
	});

	it("ignores ephemeral parameters", async () => {
		const paramEphemeral = {
			...MockPreviewParameter,
			name: "breakfast",
			display_name: "Breakfast",
			form_type: "input" as const,
			value: { value: "eggs", valid: true },
			default_value: { value: "eggs", valid: true },
			order: 0,
			ephemeral: true,
		};

		mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: 0,
						parameters: [paramRegion, paramEphemeral],
						diagnostics: [],
					}),
				}),
			);
		});

		renderEmbedPage();

		await waitFor(() => {
			expect(screen.getByDisplayValue("us-east-1")).toBeInTheDocument();
		});

		// The ephemeral parameter should be ignored. Ephemeral params don't make
		// sense in this context because they need to be reprovided on every
		// workspace start.
		expect(screen.queryByDisplayValue("eggs")).not.toBeInTheDocument();
	});

	it("includes mode and param.* params in the test link and markdown", async () => {
		const param = {
			...MockPreviewParameter,
			name: "flavor",
			display_name: "Flavor",
			form_type: "input" as const,
			value: { value: "vanilla", valid: true },
			default_value: { value: "vanilla", valid: true },
			order: 0,
		};

		mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: 0,
						parameters: [param],
						diagnostics: [],
					}),
				}),
			);
		});

		renderEmbedPage();

		// Wait for the parameter to be rendered
		await waitFor(() => {
			expect(screen.getByDisplayValue("vanilla")).toBeInTheDocument();
		});

		// The "Test" link is an <a> element; it always uses mode=manual for testing.
		const testLink = screen.getByRole("link", { name: "Test" });
		const href = testLink.getAttribute("href") ?? "";

		const searchParams = getSearchParams(href);
		expect(searchParams.get("mode")).toBe("manual");
		expect(searchParams.get("param.flavor")).toBe("vanilla");

		const copyButton = screen.getByRole("button", {
			name: /copy button markdown/i,
		});
		await userEvent.click(copyButton);

		await waitFor(() => {
			expect(writeTextMock).toHaveBeenCalled();
		});

		const copiedText = writeTextMock.mock.calls[0][0] as string;
		expect(copiedText).toContain("open-in-coder.svg");
		expect(copiedText).toContain(
			`/templates/${MockTemplate.organization_name}/${MockTemplate.name}/workspace`,
		);
		expect(copiedText).toContain("mode=manual");
		expect(copiedText).toContain("param.flavor=vanilla");
	});

	it("changes mode to auto when selected", async () => {
		mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: 0,
						parameters: [paramRegion],
						diagnostics: [],
					}),
				}),
			);
		});

		renderEmbedPage();

		await waitFor(() => {
			expect(screen.getByDisplayValue("us-east-1")).toBeInTheDocument();
		});

		// The Test link always forces mode=manual regardless of form state,
		// but the Copy button Markdown uses the selected mode.
		const autoRadio = screen.getByLabelText(/automatic/i);
		await userEvent.click(autoRadio);

		const copyButton = screen.getByRole("button", {
			name: /copy button markdown/i,
		});
		await userEvent.click(copyButton);

		await waitFor(() => {
			expect(writeTextMock).toHaveBeenCalled();
		});

		const copiedText = writeTextMock.mock.calls[0][0] as string;
		expect(copiedText).toContain("mode=auto");
		expect(copiedText).toContain("param.region=us-east-1");

		// The Test link should still use mode=manual
		const testLink = screen.getByRole("link", { name: "Test" });
		const href = testLink.getAttribute("href") ?? "";
		const searchParams = getSearchParams(href);
		expect(searchParams.get("mode")).toBe("manual");
	});

	it("sends updated values when a parameter changes", async () => {
		const [mockWebSocket] = mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: 0,
						parameters: [paramRegion],
						diagnostics: [],
					}),
				}),
			);
		});

		renderEmbedPage();

		await waitFor(() => {
			expect(screen.getByDisplayValue("us-east-1")).toBeInTheDocument();
		});

		const input = screen.getByDisplayValue("us-east-1");
		await userEvent.clear(input);
		await userEvent.type(input, "us-east-4");

		await waitFor(() => {
			expect(mockWebSocket.send).toHaveBeenCalledWith(
				expect.stringContaining('"region":"us-east-4"'),
			);
		});
	});

	it("updates form state when server responds", async () => {
		const [_, mockPublisher] = mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: 0,
						parameters: [paramRegion],
						diagnostics: [],
					}),
				}),
			);
		});

		renderEmbedPage();

		await waitFor(() => {
			expect(screen.getByDisplayValue("us-east-1")).toBeInTheDocument();
		});

		const updatedResponse: DynamicParametersResponse = {
			id: 1,
			parameters: [
				{
					...paramRegion,
					value: { value: "us-east-4", valid: true },
				},
			],
			diagnostics: [],
		};

		await act(async () => {
			// Push an updated parameter value
			mockPublisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify(updatedResponse),
				}),
			);
		});

		await waitFor(() => {
			expect(screen.getByDisplayValue("us-east-4")).toBeInTheDocument();
		});

		// Verify the Test link reflects the updated value
		const testLink = screen.getByRole("link", { name: "Test" });
		const href = testLink.getAttribute("href") ?? "";
		expect(href).toContain("param.region=us-east-4");
	});
});
