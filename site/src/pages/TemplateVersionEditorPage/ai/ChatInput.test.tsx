import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ChatInput } from "./ChatInput";

const createObjectURLMock = vi.fn();
const revokeObjectURLMock = vi.fn();

beforeAll(() => {
	Object.defineProperty(URL, "createObjectURL", {
		configurable: true,
		writable: true,
		value: createObjectURLMock,
	});
	Object.defineProperty(URL, "revokeObjectURL", {
		configurable: true,
		writable: true,
		value: revokeObjectURLMock,
	});
});

beforeEach(() => {
	createObjectURLMock.mockReset();
	revokeObjectURLMock.mockReset();
	createObjectURLMock.mockReturnValue("blob:error-preview");
});

describe("ChatInput", () => {
	it("captures pasted screenshots and sends them as attachments", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn().mockResolvedValue({ accepted: true });
		render(<ChatInput onSend={onSend} />);

		const textarea = screen.getByLabelText("Message AI assistant");
		const screenshot = new File(["image-bytes"], "error.png", {
			type: "image/png",
		});

		fireEvent.paste(textarea, {
			clipboardData: {
				files: [screenshot],
				getData: () => "",
			},
		});

		expect(screen.getByAltText("error.png")).toBeInTheDocument();

		await user.type(textarea, "Please debug this screenshot.");
		await user.click(screen.getByRole("button", { name: "Send message" }));

		await waitFor(() => {
			expect(onSend).toHaveBeenCalledWith({
				text: "Please debug this screenshot.",
				attachments: [screenshot],
			});
		});
		await waitFor(() => {
			expect(screen.queryByAltText("error.png")).not.toBeInTheDocument();
		});
		expect(textarea).toHaveValue("");
	});
});
