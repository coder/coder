import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { MockChatProject } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { ProjectMetadataBadges } from "./ProjectMetadataBadges";

describe("ProjectMetadataBadges", () => {
	it("shows the full creation time when the date receives keyboard focus", async () => {
		const user = userEvent.setup();
		renderComponent(
			<ProjectMetadataBadges
				ownerLabel="you"
				createdAt={MockChatProject.created_at}
			/>,
		);

		await user.tab();

		expect(await screen.findByRole("tooltip")).toHaveTextContent(
			formatDateTime(MockChatProject.created_at, DATE_FORMAT.FULL_DATETIME),
		);
	});
});
