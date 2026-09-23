import { expect, it } from "vitest";
import { findActiveSettingsNavigationItem } from "#/components/SettingsNavigation/types";
import { userSettingsNavigation } from "./navigation";

const labels = (showSchedulePage: boolean, showOAuth2Page: boolean) =>
	userSettingsNavigation({ showSchedulePage, showOAuth2Page }).map(
		(section) => ({
			section: section.label,
			items: section.items.map((item) => item.label),
		}),
	);

it("returns the approved user settings sections and order", () => {
	expect(labels(true, true)).toEqual([
		{
			section: "General",
			items: ["Account", "Appearance", "Notifications", "Schedule", "Security"],
		},
		{
			section: "Connected accounts",
			items: ["External authentication", "OAuth2 applications"],
		},
		{
			section: "Credentials",
			items: ["SSH keys", "Secrets", "Tokens"],
		},
	]);
});

it("applies the existing schedule and OAuth2 gates", () => {
	expect(labels(false, false)).toEqual([
		{
			section: "General",
			items: ["Account", "Appearance", "Notifications", "Security"],
		},
		{
			section: "Connected accounts",
			items: ["External authentication"],
		},
		{
			section: "Credentials",
			items: ["SSH keys", "Secrets", "Tokens"],
		},
	]);
});

it("keeps Tokens active on the create token route", () => {
	const active = findActiveSettingsNavigationItem(
		userSettingsNavigation({ showSchedulePage: true, showOAuth2Page: true }),
		"/settings/tokens/new",
	);

	expect(active?.item.id).toBe("tokens");
});
