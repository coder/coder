import { describe, expect, it } from "vitest";
import { parseLinkifiedText } from "./LinkifiedText";

describe("parseLinkifiedText", () => {
	it("links bare URLs without trailing punctuation", () => {
		expect(parseLinkifiedText("Open https://coder.com/docs.")).toEqual([
			{ type: "text", text: "Open " },
			{
				type: "link",
				text: "https://coder.com/docs",
				href: "https://coder.com/docs",
			},
			{ type: "text", text: "." },
		]);
	});

	it("keeps balanced URL parentheses", () => {
		expect(parseLinkifiedText("See https://example.com/path(foo).")).toEqual([
			{ type: "text", text: "See " },
			{
				type: "link",
				text: "https://example.com/path(foo)",
				href: "https://example.com/path(foo)",
			},
			{ type: "text", text: "." },
		]);
	});

	it("normalizes www URLs", () => {
		expect(parseLinkifiedText("Go to www.coder.com")).toEqual([
			{ type: "text", text: "Go to " },
			{
				type: "link",
				text: "www.coder.com",
				href: "https://www.coder.com",
			},
		]);
	});

	it("links markdown URLs", () => {
		expect(
			parseLinkifiedText("Read [the docs](https://coder.com/docs) now."),
		).toEqual([
			{ type: "text", text: "Read " },
			{
				type: "link",
				text: "the docs",
				href: "https://coder.com/docs",
			},
			{ type: "text", text: " now." },
		]);
	});

	it("links angle-bracket markdown URLs", () => {
		expect(
			parseLinkifiedText(
				"Open [Slack](<https://codercom.slack.com/archives/C0AGTPWLA3U/p1779700516296669?thread_ts=1779700516.296669&cid=C0AGTPWLA3U>).",
			),
		).toEqual([
			{ type: "text", text: "Open " },
			{
				type: "link",
				text: "Slack",
				href: "https://codercom.slack.com/archives/C0AGTPWLA3U/p1779700516296669?thread_ts=1779700516.296669&cid=C0AGTPWLA3U",
			},
			{ type: "text", text: "." },
		]);
	});
});
