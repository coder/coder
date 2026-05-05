/**
 * @deprecated MUI theme is deprecated. Migrate to Tailwind CSS theme system.
 *
 * Re-exports the base light MUI theme so `palette.mode === "light"` stays
 * correct for legacy MUI components. Colorblind palette overrides live in
 * `roles.ts` and the CSS variables block in `site/src/index.css`.
 */
export { default } from "../light/mui";
