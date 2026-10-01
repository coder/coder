import data from "@emoji-mart/data/sets/15/apple.json";
import EmojiMart from "@emoji-mart/react";
import { useEffect, useRef } from "react";
import { DEPRECATED_ICONS } from "#/theme/deprecatedIcons";
import icons from "#/theme/icons.json";

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

type EmojiPickerProps = Omit<
	React.ComponentProps<typeof EmojiMart>,
	"custom" | "data" | "set" | "theme" | "getSpritesheetURL"
>;

/**
 * Style overrides injected into the picker's shadow root.
 *
 * - Custom emoji images render improperly without a 100% width.
 *   Issue:   https://github.com/missive/emoji-mart/issues/805
 *   Open PR: https://github.com/missive/emoji-mart/pull/806
 * - The default dark theme renders the preview placeholder ("Pick an
 *   emoji...") and shortcodes at 45% opacity, which fails the WCAG AA
 *   4.5:1 contrast ratio. 65% opacity gives roughly 6.3:1.
 */
const pickerStyles = `
.emoji-mart-emoji img { width: 100% }
#root { --color-c: rgba(var(--em-rgb-color), .65); }
`;

const EmojiPicker: React.FC<EmojiPickerProps> = (props) => {
	const containerRef = useRef<HTMLDivElement>(null);

	// Scope the lookup to this instance, since several pickers can be mounted
	// at once (for example, the hidden preloaded one in IconField).
	useEffect(() => {
		const picker =
			containerRef.current?.querySelector("em-emoji-picker")?.shadowRoot;
		if (!picker) {
			return;
		}
		const css = document.createElement("style");
		css.textContent = pickerStyles;
		picker.appendChild(css);
	}, []);

	return (
		<div ref={containerRef} className="contents">
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
