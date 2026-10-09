import { matchRoutes, Navigate } from "react-router";
import { describe, expect, it } from "vitest";
import { router } from "./router";

describe("router", () => {
	it("redirects the old personal skills settings path to the skills page", () => {
		const match = matchRoutes(
			router.routes,
			"/agents/settings/personal-skills",
		);

		expect(match?.at(-1)?.route.element).toEqual(
			<Navigate to="/agents/settings/skills" replace />,
		);
	});
});
