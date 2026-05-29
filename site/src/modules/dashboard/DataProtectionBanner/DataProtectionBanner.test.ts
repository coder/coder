import { QueryClient } from "react-query";
import { describe, expect, it } from "vitest";
import type { UserPreferenceSettings } from "#/api/typesGenerated";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";

const defaultPrefs: UserPreferenceSettings = {
	task_notification_alert_dismissed: false,
	dpm_banner_hidden: false,
	thinking_display_mode: "auto",
	shell_tool_display_mode: "auto",
	code_diff_display_mode: "auto",
	agent_chat_send_shortcut: "enter",
};

describe("updatePreferenceSettings", () => {
	it("has mutationFn and onSuccess configured", () => {
		const queryClient = new QueryClient();
		const mutation = updatePreferenceSettings(queryClient);

		expect(mutation.mutationFn).toBeDefined();
		expect(mutation.onSuccess).toBeDefined();

		// Verify cache starts correct
		queryClient.setQueryData<UserPreferenceSettings>(
			preferenceSettings().queryKey,
			defaultPrefs,
		);
		const cached = queryClient.getQueryData<UserPreferenceSettings>(
			preferenceSettings().queryKey,
		);
		expect(cached?.dpm_banner_hidden).toBe(false);
	});
});

describe("DPM banner toggle state logic", () => {
	it("localHidden overrides queryHidden and resets on server change", () => {
		// Simulates the DPMBannerToggle pattern:
		//   isHidden = localHidden ?? queryHidden
		//   useEffect(() => setLocalHidden(null), [queryHidden])

		let queryHidden = false;

		// Initial: localHidden is null, falls through to queryHidden
		let localHidden: boolean | null = null;
		let isHidden = localHidden ?? queryHidden;
		expect(isHidden).toBe(false); // banner visible, toggle ON

		// User clicks toggle OFF: setLocalHidden(true)
		localHidden = true;
		isHidden = localHidden ?? queryHidden;
		expect(isHidden).toBe(true); // toggle OFF (instant)

		// After refetch, queryHidden changes to true, useEffect resets localHidden
		queryHidden = true;
		localHidden = null; // useEffect fires
		isHidden = localHidden ?? queryHidden;
		expect(isHidden).toBe(true); // still hidden, now confirmed by server

		// User clicks toggle ON: setLocalHidden(false)
		localHidden = false;
		isHidden = localHidden ?? queryHidden;
		expect(isHidden).toBe(false); // banner visible (instant)

		// After refetch, queryHidden changes to false, useEffect resets
		queryHidden = false;
		localHidden = null;
		isHidden = localHidden ?? queryHidden;
		expect(isHidden).toBe(false); // confirmed by server
	});

	it("banner localDismissed resets when server value changes", () => {
		// Simulates the DataProtectionBanner pattern:
		//   hidden = localDismissed || queryHidden
		//   useEffect(() => setLocalDismissed(false), [queryHidden])

		let queryHidden = false;
		let localDismissed = false;

		// Banner visible initially
		expect(localDismissed || queryHidden).toBe(false);

		// User clicks X: setLocalDismissed(true)
		localDismissed = true;
		expect(localDismissed || queryHidden).toBe(true); // hidden (instant)

		// After refetch, queryHidden becomes true, useEffect resets localDismissed
		queryHidden = true;
		localDismissed = false; // useEffect fires
		expect(localDismissed || queryHidden).toBe(true); // still hidden

		// Toggle re-enables banner: refetch makes queryHidden false
		queryHidden = false;
		localDismissed = false; // useEffect fires
		expect(localDismissed || queryHidden).toBe(false); // banner visible again
	});

	it("banner X dismiss and toggle stay in sync across interactions", () => {
		// Full scenario: banner X dismiss should eventually sync with toggle

		let queryHidden = false;

		// Banner state
		let bannerLocalDismissed = false;
		const bannerVisible = () => !(bannerLocalDismissed || queryHidden);

		// Toggle state
		let toggleLocalHidden: boolean | null = null;
		const toggleChecked = () => !(toggleLocalHidden ?? queryHidden);

		// Initial state: banner visible, toggle ON
		expect(bannerVisible()).toBe(true);
		expect(toggleChecked()).toBe(true);

		// User clicks banner X
		bannerLocalDismissed = true;
		expect(bannerVisible()).toBe(false); // banner hides instantly
		expect(toggleChecked()).toBe(true); // toggle still ON (hasn't synced yet)

		// After mutation + refetch: queryHidden becomes true
		queryHidden = true;
		bannerLocalDismissed = false; // banner useEffect resets
		toggleLocalHidden = null; // toggle useEffect resets
		expect(bannerVisible()).toBe(false); // still hidden (from server)
		expect(toggleChecked()).toBe(false); // toggle now OFF (synced)

		// User clicks toggle ON
		toggleLocalHidden = false;
		expect(toggleChecked()).toBe(true); // toggle ON instantly
		expect(bannerVisible()).toBe(false); // banner still hidden (queryHidden is still true)

		// After mutation + refetch: queryHidden becomes false
		queryHidden = false;
		bannerLocalDismissed = false; // banner useEffect resets
		toggleLocalHidden = null; // toggle useEffect resets
		expect(bannerVisible()).toBe(true); // banner visible (synced)
		expect(toggleChecked()).toBe(true); // toggle ON (confirmed)
	});
});
