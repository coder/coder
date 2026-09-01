declare module "virtual:emoji-manifest" {
	type EmojiManifest = import("../../scripts/update-emojis.mjs").EmojiManifest;
	type SkinVariation = import("../../scripts/update-emojis.mjs").SkinVariation;

	const manifest: EmojiManifest;

	export type { SkinVariation };
	export default manifest;
}
