import data from "@emoji-mart/data/sets/15/apple.json";
import EmojiMart from "@emoji-mart/react";
import {
	type ComponentProps,
	type FC,
	useEffect,
	useLayoutEffect,
} from "react";
import icons from "theme/icons.json";

const custom = [
	{
		id: "icons",
		name: "Icons",
		emojis: icons.map((icon) => {
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
	ComponentProps<typeof EmojiMart>,
	"custom" | "data" | "set" | "theme"
>;

const EmojiPicker: FC<EmojiPickerProps> = (props) => {
	/**
	 * Workaround for a bug in the emoji-mart library where custom emoji images render improperly.
	 * Setting the image width to 100% ensures they display correctly.
	 *
	 * Issue:   https://github.com/missive/emoji-mart/issues/805
	 * Open PR: https://github.com/missive/emoji-mart/pull/806
	 */
	useEffect(() => {
		const picker = document.querySelector("em-emoji-picker")?.shadowRoot;
		if (!picker) {
			return;
		}
		const css = document.createElement("style");
		css.textContent = ".emoji-mart-emoji img { width: 100% }";
		picker.appendChild(css);
	}, []);

	return (
		<EmojiMart
			theme="dark"
			set="apple"
			emojiVersion="15"
			data={data}
			custom={custom}
			{...props}
		/>
	);
};

export default EmojiPicker;
