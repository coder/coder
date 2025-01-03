import data from "@emoji-mart/data/sets/15/apple.json";
import EmojiMart from "@emoji-mart/react";
import type { ComponentProps, FC } from "react";
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
