declare module "virtual:emoji-manifest" {
	type EmojiManifest = import("../../scripts/update-emojis.mjs").EmojiManifest;
	type SheetPosition = import("../../scripts/update-emojis.mjs").SheetPosition;
	type SkinVariation = import("../../scripts/update-emojis.mjs").SkinVariation;

	const manifest: EmojiManifest;

	export type { SheetPosition, SkinVariation };
	export default manifest;
}
