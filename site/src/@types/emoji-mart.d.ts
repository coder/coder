declare module "@emoji-mart/react" {
  const Picker: React.FC<{
    theme: "dark" | "light";
    data: Record<string, unknown>;
    onEmojiSelect: (emojiData: { unified: string }) => void;
  }>;

  export default Picker;
}
