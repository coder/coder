import data from "@emoji-mart/data/sets/15/apple.json";
import EmojiMart from "@emoji-mart/react";
import { useEffect, useRef } from "react";
import icons from "#/theme/builtinIcons.json";
import { DEPRECATED_ICONS } from "#/theme/deprecatedIcons";

const custom = [
	{
		id: "icons",
		name: "Icons",
		emojis: icons.flatMap((icon) =>
			icon.sources
				.filter((src) => !DEPRECATED_ICONS.includes(src.slice("/icon/".length)))
				.map((src) => {
					const id = src.slice("/icon/".length).replace(/\.[^.]+$/, "");
					return {
						id,
						name: icon.name,
						keywords: [...icon.name.split(/\s+/), ...id.split("-")],
						skins: [{ src }],
					};
				}),
		),
	},
];

type EmojiPickerProps = Omit<
	React.ComponentProps<typeof EmojiMart>,
	"custom" | "data" | "set" | "theme" | "getSpritesheetURL"
>;

const EmojiPicker: React.FC<EmojiPickerProps> = (props) => {
	const ref = useRef<HTMLDivElement>(null);

	/**
	 * Workaround for a bug in the emoji-mart library where custom emoji images render improperly.
	 * Setting the image width to 100% ensures they display correctly.
	 *
	 * Issue:   https://github.com/missive/emoji-mart/issues/805
	 * Open PR: https://github.com/missive/emoji-mart/pull/806
	 *
	 * Also raise the dark theme's 45% opacity secondary text ("Pick an emoji"
	 * placeholder) to 65% so it meets the WCAG AA 4.5:1 contrast ratio.
	 *
	 * Query within this instance, since IconField also mounts a hidden picker.
	 */
	useEffect(() => {
		const picker = ref.current?.querySelector("em-emoji-picker")?.shadowRoot;
		if (!picker) {
			return;
		}
		const css = document.createElement("style");
		css.textContent =
			".emoji-mart-emoji img { width: 100% } #root { --color-c: rgba(var(--em-rgb-color), .65) }";
		picker.appendChild(css);
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
