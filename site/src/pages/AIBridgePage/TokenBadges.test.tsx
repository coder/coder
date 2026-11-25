import { screen } from "@testing-library/react";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { TokenBadges } from "./TokenBadges";

describe("TokenBadges", () => {
	it("names each token count instead of relying on the arrow icons", () => {
		renderComponent(<TokenBadges inputTokens={1200} outputTokens={34} />);

		expect(screen.getByRole("button")).toHaveAccessibleName(
			/Input tokens:\s*1\.2K\s+Output tokens:\s*34/,
		);
	});
});
