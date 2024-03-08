import data from "@emoji-mart/data/sets/14/twitter.json";
import EmojiMart, { type EmojiMartProps } from "@emoji-mart/react";
import type { FC } from "react";
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
  EmojiMartProps,
  "custom" | "data" | "set" | "theme"
>;

const EmojiPicker: FC<EmojiPickerProps> = (props) => {
  return (
    <EmojiMart
      theme="dark"
      set="twitter"
      data={data}
      custom={custom}
      {...props}
    />
  );
};

export default EmojiPicker;
