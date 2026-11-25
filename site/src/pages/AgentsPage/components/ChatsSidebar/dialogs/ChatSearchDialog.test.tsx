import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, useLocation } from "react-router";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { ChatSearchDialog } from "./ChatSearchDialog";

const LocationProbe: React.FC = () => {
	const location = useLocation();
	return <div data-testid="location-pathname">{location.pathname}</div>;
};

describe("ChatSearchDialog", () => {
	it("keeps the chosen chat selected when the recent chats reorder", async () => {
		const user = userEvent.setup();
		const login: Chat = { ...MockChat, id: "chat-login", title: "Fix login" };
		const docs: Chat = { ...MockChat, id: "chat-docs", title: "Write docs" };
		const queryClient = createTestQueryClient();
		const dialog = (recentChats: Chat[]) => (
			<QueryClientProvider client={queryClient}>
				<MemoryRouter initialEntries={["/agents"]}>
					<ChatSearchDialog
						open
						onOpenChange={vi.fn()}
						location={{
							pathname: "/agents",
							search: "",
							hash: "",
							state: null,
							key: "default",
						}}
						recentChats={recentChats}
					/>
					<LocationProbe />
				</MemoryRouter>
			</QueryClientProvider>
		);
		const { rerender } = render(dialog([login, docs]));

		await user.click(screen.getByRole("combobox", { name: "Search chats" }));
		await user.keyboard("{ArrowDown}{ArrowDown}");
		rerender(dialog([docs, login]));
		await user.keyboard("{Enter}");

		expect(screen.getByTestId("location-pathname")).toHaveTextContent(
			"/agents/chat-docs",
		);
	});
});
