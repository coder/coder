/**
 * @deprecated MUI theme is deprecated. Migrate to Tailwind CSS theme system.
 *
 * MUI components are deprecated and the colorblind-friendly palette is
 * expressed through `roles.ts` and the CSS variables in
 * `site/src/index.css`, both of which drive the Tailwind-rendered UI that
 * the diff panel and semantic roles use. We re-export the base dark MUI
 * theme so `palette.mode === "dark"` stays correct for any remaining
 * legacy MUI component that inspects it (for example, the Shiki theme
 * selector in `DiffViewer.tsx`).
 */
export { default } from "../dark/mui";
