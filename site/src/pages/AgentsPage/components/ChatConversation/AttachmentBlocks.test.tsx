import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { AttachmentBlock } from "./AttachmentBlocks";
import { FileProbeProvider } from "./FileProbeContext";

describe("AttachmentBlock", () => {
	it("aborts a text preview request when the file expires", async () => {
		const user = userEvent.setup();
		let requestSignal: AbortSignal | undefined;
		vi.spyOn(window, "fetch").mockImplementation((_input, init) => {
			requestSignal = init?.signal ?? undefined;
			return new Promise(() => {});
		});
		const attachment = (evictedFileIds: ReadonlySet<string>) => (
			<FileProbeProvider evictedFileIds={evictedFileIds}>
				<AttachmentBlock
					block={{
						type: "file",
						media_type: "text/plain",
						file_id: "notes",
						name: "notes.txt",
					}}
					onImageClick={vi.fn()}
					onTextFileClick={vi.fn()}
				/>
			</FileProbeProvider>
		);
		const { rerender } = renderComponent(attachment(new Set()));

		await user.click(screen.getByRole("button", { name: "View notes.txt" }));
		await waitFor(() => expect(requestSignal).toBeDefined());

		rerender(attachment(new Set(["notes"])));
		expect(requestSignal?.aborted).toBe(true);
	});
});
