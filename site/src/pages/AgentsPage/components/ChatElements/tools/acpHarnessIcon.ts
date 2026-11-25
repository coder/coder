const harnessIcons: readonly (readonly [RegExp, string])[] = [
	[/claude/i, "/icon/claude.svg"],
	[/codex/i, "/icon/openai-codex.svg"],
	[/gemini/i, "/icon/gemini.svg"],
	[/copilot/i, "/icon/github-copilot.svg"],
	[/open[\s_-]?code/i, "/icon/opencode.svg"],
	[/goose/i, "/icon/goose.svg"],
	[/(?:^|[^a-z0-9])amp(?:$|[^a-z0-9])/i, "/icon/amp.svg"],
	[/aider/i, "/icon/aider.svg"],
	[/cursor/i, "/icon/cursor.svg"],
	[/kiro/i, "/icon/kiro.svg"],
	[/auggie/i, "/icon/auggie.svg"],
	[/crush/i, "/icon/charm-crush.svg"],
	[/junie/i, "/icon/junie.svg"],
	[/kilo/i, "/icon/kilo-code.svg"],
	[/roo[\s_-]?code/i, "/icon/roo-code.svg"],
	[/amazon[\s_-]?q/i, "/icon/amazon-q.svg"],
	[/devin/i, "/icon/devin.svg"],
	[/(?:^|[^a-z0-9])pi(?:$|[^a-z0-9])/i, "/icon/pi.svg"],
	[/windsurf/i, "/icon/windsurf.svg"],
	[/(?:^|[^a-z0-9])zed(?:$|[^a-z0-9])/i, "/icon/zed.svg"],
];

/** Matches known clients by harness ID first, then by display name. */
export function getACPHarnessIcon(
	harnessId: string,
	displayName: string,
): string | undefined {
	for (const value of [harnessId, displayName]) {
		const match = harnessIcons.find(([pattern]) => pattern.test(value));
		if (match) return match[1];
	}
	return undefined;
}
