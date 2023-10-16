declare module "@emoji-mart/react" {
  interface CustomCategory {
    id: string;
    name: string;
    emojis: CustomEmoji[];
  }

  interface CustomEmoji {
    id: string;
    name: string;
    keywords: string[];
    skins: CustomEmojiSkin[];
  }

  interface CustomEmojiSkin {
    src: string;
  }

  type EmojiData = EmojiResource & {
    id: string;
    keywords: string[];
    name: string;
    native?: string;
    shortcodes: string;
  };

  type EmojiResource =
    | { unified: undefined; src: string }
    | { unified: string; src: undefined };

  const EmojiPicker: React.FC<{
    set: "native" | "apple" | "facebook" | "google" | "twitter";
    theme: "dark" | "light";
    data: unknown;
    custom: CustomCategory[];
    emojiButtonSize?: number;
    emojiSize?: number;
    onEmojiSelect: (emoji: EmojiData) => void;
  }>;

  export default EmojiPicker;
}
