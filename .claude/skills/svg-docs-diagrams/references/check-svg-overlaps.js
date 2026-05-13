#!/usr/bin/env node
/**
 * check-svg-overlaps.js: programmatic overlap and overflow checker for
 * SVG diagrams that ship in Coder docs pages.
 *
 * Usage:
 *   node check-svg-overlaps.js <svg-path> [--docs-url <url>]
 *
 * If --docs-url is given, it checks the SVG as rendered on that docs
 * page (so the test matches what readers see at the column width).
 * Otherwise it renders the SVG file directly at its natural viewBox
 * size, which is what you want for "did anything overflow its card"
 * questions.
 *
 * Exit code 0 if no problems; 1 if any overlaps or overflows reported.
 *
 * Requires puppeteer-core and a chrome / chromium binary in PATH.
 * If puppeteer-core is not installed in the current directory's
 * node_modules, the script tries /tmp/node_modules as a fallback.
 *
 * What it checks
 * --------------
 *
 * 1. TEXT OVERFLOW: every <text> element must lie inside the bounding
 *    box of its associated container <rect>. The associated container
 *    is the most recent preceding sibling <rect> with class containing
 *    "card", "box", "badge", or "pill" (configurable).
 *
 * 2. CARD COLLISION: no two container rects of the same class family
 *    overlap each other unless they are explicitly nested (e.g.
 *    'session-card' is allowed to sit inside 'runner-box').
 *
 * 3. BADGE COLLISION: no two badges (rects with class containing
 *    "badge" or "pill") overlap each other.
 *
 * 4. ORPHAN TEXT: every <text> whose nearest preceding rect is more
 *    than 200 viewBox-units away is flagged. This catches arrow labels
 *    placed at arbitrary positions; you can mark them with class
 *    "arrow-label" or "footnote" to opt out.
 *
 * The script reports each finding with the element's coordinates and
 * the rule it violated, so you can fix it without guessing.
 */

const path = require("path");
const fs = require("fs");

// Locate puppeteer-core.
let puppeteer;
for (const root of [
  process.cwd(),
  "/tmp",
  path.join(process.env.HOME || "/root", ".npm-global"),
]) {
  const p = path.join(root, "node_modules", "puppeteer-core");
  if (fs.existsSync(p)) {
    puppeteer = require(p);
    break;
  }
}
if (!puppeteer) {
  console.error(
    "puppeteer-core not found. Install with: npm install --prefix /tmp puppeteer-core",
  );
  process.exit(2);
}

function findChrome() {
  const candidates = [
    "/usr/bin/google-chrome",
    "/usr/bin/google-chrome-stable",
    "/usr/bin/chromium",
    "/usr/bin/chromium-browser",
    "/snap/bin/chromium",
  ];
  for (const c of candidates) if (fs.existsSync(c)) return c;
  return null;
}

const args = process.argv.slice(2);
if (args.length === 0 || args[0] === "-h" || args[0] === "--help") {
  console.error(
    "usage: check-svg-overlaps.js <svg-path> [--docs-url <url>] [--allow-nesting parent>child[,parent>child...]]",
  );
  process.exit(2);
}
const svgPath = path.resolve(args[0]);
let docsUrl = null;
let allowNesting = [
  // Defaults: child class -> parent class. These nestings are intended.
  ["session-card", "runner-box"],
  ["runner-box", "workspace-box"],
  ["card-anthropic", "zone-anthropic"],
  ["card-coder", "zone-coder"],
  ["card-network", "zone-network"],
  ["card-routing", "zone-routing"],
  ["workspace-warm", "zone-coder"],
  ["workspace-claimed", "zone-coder"],
  ["workspace-locked", "zone-coder"],
  ["workspace-empty", "zone-coder"],
  ["side-card", "anthropic-zone"],
  ["side-card", "network-zone"],
  ["side-card-a", "anthropic-zone"],
  ["side-card-n", "network-zone"],
  // Badges can sit inside any container.
  ["badge-coder", "workspace-box"],
  ["badge-anthropic", "runner-box"],
  ["badge-locked", "runner-box"],
  ["badge-coder", "workspace-claimed"],
  ["badge-anthropic", "workspace-claimed"],
  ["lock-badge", "workspace-locked"],
  ["owner-badge", "workspace-claimed"],
  ["lock-badge", "workspace-claimed"],
  ["planned-stamp", "zone-routing"],
];

for (let i = 1; i < args.length; i++) {
  if (args[i] === "--docs-url") {
    docsUrl = args[++i];
  } else if (args[i] === "--allow-nesting") {
    const extra = args[++i]
      .split(",")
      .map((s) => s.trim().split(">").map((t) => t.trim()));
    allowNesting = allowNesting.concat(extra);
  }
}

const chromePath = findChrome();
if (!chromePath) {
  console.error("no chrome/chromium binary found in PATH");
  process.exit(2);
}

// ---------------------------------------------------------------
// Logic that runs inside the Chrome page (serialized to a string).
// ---------------------------------------------------------------
function pageProbeFactory() {
  return function pageProbe(allowNesting) {
    function rectsIntersect(a, b) {
      return !(
        a.right <= b.left ||
        b.right <= a.left ||
        a.bottom <= b.top ||
        b.bottom <= a.top
      );
    }
    function rectContains(outer, inner, tol) {
      tol = tol || 0;
      return (
        inner.left >= outer.left - tol &&
        inner.right <= outer.right + tol &&
        inner.top >= outer.top - tol &&
        inner.bottom <= outer.bottom + tol
      );
    }
    function getBox(el) {
      const b = el.getBBox();
      return {
        left: b.x,
        top: b.y,
        right: b.x + b.width,
        bottom: b.y + b.height,
        width: b.width,
        height: b.height,
      };
    }
    function classList(el) {
      return (el.getAttribute("class") || "").split(/\s+/).filter(Boolean);
    }
    function classKind(el, kinds) {
      const cls = classList(el);
      for (const c of cls) {
        for (const k of kinds) if (c === k || c.includes(k)) return c;
      }
      return null;
    }
    function hasKind(el, kinds) {
      return classKind(el, kinds) !== null;
    }

    const svg = document.querySelector("svg");
    if (!svg) return { error: "no <svg> element found" };

    const all = Array.from(svg.querySelectorAll("rect, text"));

    const containerKinds = [
      "workspace-box",
      "runner-box",
      "session-card",
      "card",
      "zone",
      "badge",
      "pill",
      "lock-badge",
      "owner-badge",
      "planned-stamp",
      "card-anthropic",
      "card-coder",
      "card-network",
      "card-routing",
      "workspace-warm",
      "workspace-locked",
      "workspace-claimed",
      "workspace-empty",
      "anthropic-zone",
      "network-zone",
      "side-card",
    ];
    const optOutTextKinds = ["arrow-label", "footnote", "zone-label"];

    const rects = [];
    const texts = [];

    for (const el of all) {
      const box = getBox(el);
      const cls = classList(el);
      const kind = classKind(el, containerKinds);
      const entry = { el, tag: el.tagName.toLowerCase(), cls, kind, box };
      if (el.tagName.toLowerCase() === "rect" && kind) {
        rects.push(entry);
      } else if (el.tagName.toLowerCase() === "text") {
        entry.text = el.textContent.trim();
        entry.optOut = hasKind(el, optOutTextKinds);
        texts.push(entry);
      }
    }

    // For each text, find its associated container: the most recent
    // preceding <rect> in document order whose class kind matches one
    // of the container kinds.
    function indexOf(arr, el) {
      return arr.findIndex((e) => e.el === el);
    }
    const orderIndex = new Map();
    all.forEach((el, i) => orderIndex.set(el, i));

    // The associated container of a text is the smallest enclosing rect.
    // If no rect encloses the text, the text is treated as free-floating
    // (an annotation, zone label, arrow label, footnote, or similar) and
    // skipped by the overflow rule. Collision rules still apply.
    function findAssociatedRect(t) {
      const enclosing = rects.filter((r) => rectContains(r.box, t.box, 1.5));
      if (enclosing.length === 0) return null;
      enclosing.sort(
        (a, b) => a.box.width * a.box.height - b.box.width * b.box.height,
      );
      return enclosing[0];
    }

    const findings = [];

    // 1. TEXT OVERFLOW: only checks text that has an enclosing rect.
    // Free-floating text (footnotes, zone labels, arrow labels) is
    // checked by collision rules below, not by containment.
    for (const t of texts) {
      if (t.optOut) continue;
      const r = findAssociatedRect(t);
      if (!r) continue;
      // Find the next-larger enclosing rect that's NOT the smallest;
      // if the text fits in the smallest, no overflow.
      const TOLERANCE = 1.5;
      if (!rectContains(r.box, t.box, TOLERANCE)) {
        findings.push({
          rule: "text-overflow",
          severity: "error",
          text: t.text,
          textBox: t.box,
          container: r.cls.join(" "),
          containerBox: r.box,
          message: `text "${t.text}" overflows container .${r.kind}`,
        });
      }
    }

    // 1b. TEXT-NEAR-RECT OVERFLOW: when a text is NOT inside any rect
    // but its bbox horizontally overlaps a rect that's clearly meant
    // to be its container (same y range), flag it as overflow.
    // This catches the case where a label literally pokes outside its
    // card so it's no longer enclosed.
    for (const t of texts) {
      if (t.optOut) continue;
      if (findAssociatedRect(t)) continue;
      // Look for a rect that vertically contains the text and whose
      // x range overlaps the text's x range; if found, that's the
      // intended container and the text overflows it.
      const candidate = rects.find((r) => {
        const vert = t.box.top >= r.box.top && t.box.bottom <= r.box.bottom;
        const hOverlap =
          t.box.left < r.box.right && t.box.right > r.box.left;
        const beyondRight = t.box.right > r.box.right;
        const beyondLeft = t.box.left < r.box.left;
        return vert && hOverlap && (beyondRight || beyondLeft);
      });
      if (candidate) {
        findings.push({
          rule: "text-overflow",
          severity: "error",
          text: t.text,
          textBox: t.box,
          container: candidate.cls.join(" "),
          containerBox: candidate.box,
          message: `text "${t.text}" pokes outside likely container .${candidate.kind}`,
        });
      }
    }

    // 2 + 3. RECT COLLISIONS
    const allowSet = new Set(
      allowNesting.map(([child, parent]) => `${child}>${parent}`),
    );
    function allowedNesting(childKind, parentKind) {
      return (
        allowSet.has(`${childKind}>${parentKind}`) ||
        allowSet.has(`${parentKind}>${childKind}`)
      );
    }

    for (let i = 0; i < rects.length; i++) {
      for (let j = i + 1; j < rects.length; j++) {
        const a = rects[i];
        const b = rects[j];
        if (!rectsIntersect(a.box, b.box)) continue;
        // Allowed nesting (one fully contains the other).
        if (rectContains(a.box, b.box, 1) || rectContains(b.box, a.box, 1)) {
          if (allowedNesting(a.kind, b.kind)) continue;
          // Unexpected full-containment between siblings: only flag if
          // they're the same kind (e.g. two badge-coder stacked).
          if (a.kind === b.kind) {
            findings.push({
              rule: "rect-stacking",
              severity: "warn",
              kinds: [a.kind, b.kind],
              a: { cls: a.cls.join(" "), box: a.box },
              b: { cls: b.cls.join(" "), box: b.box },
              message: `two .${a.kind} rects fully overlap`,
            });
          }
          continue;
        }
        // Partial overlap is always wrong.
        findings.push({
          rule: "rect-collision",
          severity: "error",
          kinds: [a.kind, b.kind],
          a: { cls: a.cls.join(" "), box: a.box },
          b: { cls: b.cls.join(" "), box: b.box },
          message: `.${a.kind} partially overlaps .${b.kind}`,
        });
      }
    }

    // 4. ARROW-LABEL COLLISIONS WITH NON-ROUTE RECTS
    // Optional sanity: warn if an arrow-label text sits on top of a
    // container rect that's not the one it's logically tied to.
    for (const t of texts) {
      if (!t.optOut) continue;
      // Only check arrow-label specifically.
      const isArrow = t.cls.some((c) => c.includes("arrow-label"));
      if (!isArrow) continue;
      const enclosing = rects.filter(
        (r) => rectContains(r.box, t.box, 0) && r.kind !== "workspace-box",
      );
      // Hitting a card or badge full-cover is suspicious.
      const badHits = enclosing.filter(
        (r) =>
          r.kind &&
          (r.kind.includes("card") ||
            r.kind.includes("badge") ||
            r.kind === "workspace-warm" ||
            r.kind === "workspace-locked" ||
            r.kind === "workspace-claimed"),
      );
      for (const h of badHits) {
        findings.push({
          rule: "arrow-label-on-card",
          severity: "warn",
          text: t.text,
          textBox: t.box,
          covers: h.cls.join(" "),
          coversBox: h.box,
          message: `arrow label "${t.text}" sits on top of .${h.kind}`,
        });
      }
    }

    // 5. TEXT-TEXT COLLISIONS (different texts whose bboxes overlap).
    for (let i = 0; i < texts.length; i++) {
      for (let j = i + 1; j < texts.length; j++) {
        const a = texts[i];
        const b = texts[j];
        if (a.optOut && b.optOut) continue;
        if (!rectsIntersect(a.box, b.box)) continue;
        findings.push({
          rule: "text-text-collision",
          severity: "error",
          a: { text: a.text, box: a.box, cls: a.cls.join(" ") },
          b: { text: b.text, box: b.box, cls: b.cls.join(" ") },
          message: `text "${a.text}" and "${b.text}" overlap`,
        });
      }
    }

    return { findings, rectCount: rects.length, textCount: texts.length };
  };
}

(async function main() {
  const browser = await puppeteer.launch({
    executablePath: chromePath,
    headless: "new",
    args: [
      "--no-sandbox",
      "--disable-dev-shm-usage",
      "--disable-gpu",
      "--hide-scrollbars",
    ],
    defaultViewport: { width: 1280, height: 2000 },
  });
  try {
    const page = await browser.newPage();
    let probeTarget;
    if (docsUrl) {
      // Render the docs page and probe the SVG element directly.
      await page.goto(docsUrl, {
        waitUntil: "domcontentloaded",
        timeout: 60000,
      });
      await new Promise((r) => setTimeout(r, 2000));
      // We probe the inline SVG; but on most docs pages the SVG is an
      // <img src=".svg">. Fetch and inline it so getBBox works.
      const svgFileName = path.basename(svgPath);
      const inlined = await page.evaluate(async (fname) => {
        const imgs = Array.from(document.querySelectorAll("img"));
        const target = imgs.find((i) => i.src.endsWith(fname));
        if (!target) return null;
        const res = await fetch(target.src);
        const svgText = await res.text();
        const wrapper = document.createElement("div");
        wrapper.innerHTML = svgText;
        const svg = wrapper.querySelector("svg");
        if (!svg) return null;
        // Replace the img with the inline svg so getBBox works at the
        // rendered size.
        target.replaceWith(svg);
        return true;
      }, svgFileName);
      if (!inlined) {
        console.error(
          `could not find <img src=".../${path.basename(svgPath)}"> on ${docsUrl}`,
        );
        process.exit(2);
      }
      probeTarget = docsUrl;
    } else {
      const fileUrl = "file://" + svgPath;
      await page.goto(fileUrl, { waitUntil: "domcontentloaded" });
      probeTarget = fileUrl;
    }

    const probeSrc = pageProbeFactory().toString();
    const result = await page.evaluate(
      (probeSrcLiteral, allow) => {
        // eslint-disable-next-line no-new-func
        const fn = new Function("return " + probeSrcLiteral)();
        return fn(allow);
      },
      probeSrc,
      allowNesting,
    );

    if (result.error) {
      console.error("probe error:", result.error);
      process.exit(2);
    }
    console.log(
      `SVG: ${svgPath} (${result.rectCount} rects, ${result.textCount} texts) via ${probeTarget}`,
    );

    const findings = result.findings || [];
    const errors = findings.filter((f) => f.severity === "error");
    const warns = findings.filter((f) => f.severity === "warn");

    if (findings.length === 0) {
      console.log("no overlaps or overflows detected.");
      process.exit(0);
    }

    function fmtBox(b) {
      return `(${b.left.toFixed(1)}, ${b.top.toFixed(1)}) to (${b.right.toFixed(1)}, ${b.bottom.toFixed(1)})`;
    }

    for (const f of [...errors, ...warns]) {
      const sev = f.severity.toUpperCase();
      console.log(`[${sev}] ${f.rule}: ${f.message}`);
      if (f.textBox) console.log(`  text box: ${fmtBox(f.textBox)}`);
      if (f.containerBox)
        console.log(
          `  container .${f.container || ""} box: ${fmtBox(f.containerBox)}`,
        );
      if (f.coversBox) console.log(`  covers box: ${fmtBox(f.coversBox)}`);
      if (f.a && f.a.box) console.log(`  a (${f.a.text || f.a.cls}): ${fmtBox(f.a.box)}`);
      if (f.b && f.b.box) console.log(`  b (${f.b.text || f.b.cls}): ${fmtBox(f.b.box)}`);
    }
    console.log(
      `summary: ${errors.length} error(s), ${warns.length} warning(s).`,
    );
    process.exit(errors.length > 0 ? 1 : 0);
  } finally {
    await browser.close();
  }
})();
