import { describe, expect, it } from "vitest";
import { userIdentity } from "./userIdentity";

describe("userIdentity", () => {
	it("uses username as title, email as subtitle, and avatar_url as src", () => {
		expect(
			userIdentity({
				username: "aqandrew",
				email: "andrew@coder.com",
				avatar_url: "https://avatars.githubusercontent.com/u/9038965?v=4",
			}),
		).toEqual({
			title: "aqandrew",
			subtitle: "andrew@coder.com",
			src: "https://avatars.githubusercontent.com/u/9038965?v=4",
		});
	});

	it("uses Service Account subtitle for service accounts", () => {
		expect(
			userIdentity({
				username: "bot",
				email: "",
				is_service_account: true,
			}),
		).toEqual({
			title: "bot",
			subtitle: "Service Account",
			src: undefined,
		});
	});

	it("prefers Service Account subtitle over a populated email", () => {
		expect(
			userIdentity({
				username: "bot",
				email: "bot@coder.com",
				is_service_account: true,
			}).subtitle,
		).toBe("Service Account");
	});

	it("omits subtitle and src when empty", () => {
		expect(
			userIdentity({ username: "bob", email: "", avatar_url: "" }),
		).toEqual({
			title: "bob",
			subtitle: undefined,
			src: undefined,
		});
	});
});
