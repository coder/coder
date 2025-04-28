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

	export interface EmojiMartProps {
		set: "native" | "apple" | "facebook" | "google" | "twitter";
		theme: "dark" | "light";
		data: unknown;
		custom: CustomCategory[];
		emojiButtonSize?: number;
		emojiSize?: number;
		emojiVersion?: string;
		onEmojiSelect: (emoji: EmojiData) => void;
	}

	const EmojiMart: React.FC<EmojiMartProps>;

	export default EmojiMart;
}
