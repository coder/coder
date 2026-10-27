# Coder Kit

`kit/` is a verbatim copy of `skills/coder-brand/kit/` in
[coder/fux-plugins](https://github.com/coder/fux-plugins) at commit
`1c95d3edb28b3be8feaf161b8882cb3dbbe30363` (2026-10-23): `coder-kit.css`,
`components.css`, `icons.css`, `coder-kit.js` and the two fonts. The design
team edits the Coder Kit Product Design System in Claude Design, and the
coder-brand skill follows it. To update, copy the same files from a newer
commit and change the commit above. Do not edit the copies by hand.

`SKILL.md` is the built-in `coder-kit` skill for chat agents. It condenses
`SKILL.md`, `references/product-ui.md` and `references/components.md` of the
coder-brand skill for pages that coderd serves.

Licenses:

- `kit/fonts/Geist-Variable.woff2` and `kit/fonts/GeistMono-Variable.woff2`
  are Geist and Geist Mono 1.8.2 from the `geist` npm package, Copyright (c)
  2023 Vercel, in collaboration with basement.studio, under the SIL Open Font
  License 1.1 (`kit/fonts/OFL.txt`).
- `kit/icons.css` holds icons from [Lucide](https://lucide.dev)
  (`lucide-static` 1.61.0) as CSS masks. Lucide is under the ISC License, and
  the icons it took from Feather are under the MIT License
  (`kit/LICENSE-lucide.txt`).
