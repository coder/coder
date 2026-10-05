import data from "@emoji-mart/data/sets/15/apple.json";
import EmojiMart from "@emoji-mart/react";
import { useEffect, useRef } from "react";
import { DEPRECATED_ICONS } from "#/theme/deprecatedIcons";
import icons from "#/theme/icons.json";
import { enableEmojiGridNavigation } from "./emojiGridNavigation";

const custom = [
	{
		id: "icons",
		name: "Icons",
		emojis: icons
			.filter((icon) => !DEPRECATED_ICONS.includes(icon))
			.map((icon) => {
				const id = icon.split(".")[0];

				return {
					id,
					name: id,
					keywords: id.split("-"),
					skins: [{ src: `/icon/${icon}` }],
				};
			}),
	},
];

/**
 * Styles injected into the emoji-mart shadow root.
 *
 * - Custom emoji images render improperly without a 100% width.
 *   Issue:   https://github.com/missive/emoji-mart/issues/805
 *   Open PR: https://github.com/missive/emoji-mart/pull/806
 * - Raise the dark theme's 45% opacity secondary text ("Pick an emoji"
 *   placeholder) to 65% so it meets the WCAG AA 4.5:1 contrast ratio.
 * - The library has no visible focus indicator. Buttons (category nav,
 *   emojis, skin tone) and skin tone options get a focus ring, and the emoji
 *   highlighted by arrow key navigation from the search input, which keeps
 *   DOM focus, gets the same ring instead of only a faint background. The
 *   accent color is about 4.9:1 against the dark picker background.
 */
const shadowStyles = `
.emoji-mart-emoji img { width: 100% }
#root { --color-c: rgba(var(--em-rgb-color), .65) }
button:focus-visible,
.menu input[type="radio"]:focus-visible + .option,
.category button[data-keyboard][aria-selected] {
	outline: 2px solid rgb(var(--em-rgb-accent));
	outline-offset: -2px;
}
`;

type EmojiPickerProps = Omit<
	React.ComponentProps<typeof EmojiMart>,
	"custom" | "data" | "set" | "theme" | "getSpritesheetURL"
>;

const EmojiPicker: React.FC<EmojiPickerProps> = (props) => {
	const ref = useRef<HTMLDivElement>(null);

	// Query within this instance, since IconField also mounts a hidden picker.
	useEffect(() => {
		const picker = ref.current?.querySelector("em-emoji-picker")?.shadowRoot;
		if (!picker) {
			return;
		}
		const css = document.createElement("style");
		css.textContent = shadowStyles;
		picker.appendChild(css);
		return enableEmojiGridNavigation(picker);
	}, []);

	return (
		<div ref={ref} className="contents">
			<EmojiMart
				theme="dark"
				set="apple"
				emojiVersion="15"
				data={data}
				custom={custom}
				getSpritesheetURL={() => "/emojis/spritesheet.png"}
				{...props}
			/>
		</div>
	);
};

export default EmojiPicker;
