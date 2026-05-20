const DISMISSED_KEY = "agents.onboarding-dismissed";

export function isOnboardingDismissed(): boolean {
	try {
		return localStorage.getItem(DISMISSED_KEY) === "true";
	} catch {
		return false;
	}
}

export function dismissOnboarding(): void {
	try {
		localStorage.setItem(DISMISSED_KEY, "true");
	} catch {
		// localStorage may be unavailable in some contexts.
	}
}
