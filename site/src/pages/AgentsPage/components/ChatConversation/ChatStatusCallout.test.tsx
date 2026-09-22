import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ChatStatusCallout } from "./ChatStatusCallout";
import type { LiveStatusModel } from "./liveStatusModel";

const failedStatus: LiveStatusModel = {
	phase: "failed",
	hasAccumulatedOutput: false,
	title: "Authentication failed",
	kind: "auth",
	message:
		"AWS Bedrock is still setting up the Marketplace subscription for this model. Wait a few minutes and try again.",
	detail: "Your subscription to the model is being set up.",
	provider: "bedrock",
	statusCode: 403,
};

describe("ChatStatusCallout", () => {
	it("invokes onRetry when the user clicks try again", async () => {
		const user = userEvent.setup();
		const onRetry = vi.fn();
		render(<ChatStatusCallout status={failedStatus} onRetry={onRetry} />);

		await user.click(screen.getByRole("button", { name: "Try again" }));

		expect(onRetry).toHaveBeenCalledTimes(1);
	});

	it("hides try again when no retry handler is available", () => {
		render(<ChatStatusCallout status={failedStatus} />);

		expect(
			screen.queryByRole("button", { name: "Try again" }),
		).not.toBeInTheDocument();
	});
});
