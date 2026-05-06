import { type FC, useEffect, useRef } from "react";
import { ROSTER } from "./roster";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Point {
	x: number;
	y: number;
}

interface LandingPad {
	x: number;
	width: number;
	y: number;
	multiplier: number;
	isStation: boolean;
}

interface Terrain {
	points: Point[];
	pads: LandingPad[];
}

interface ExplosionParticle {
	x: number;
	y: number;
	vx: number;
	vy: number;
	len: number;
	angle: number;
	spin: number;
}

interface Codernaut {
	x: number;
	padIdx: number;
	dir: 1 | -1;
	speed: number;
	walkPhase: number;
	waving: boolean;
	waveTimer: number;
	wavePhase: number;
	nextWave: number;
	name: string;
	role: string;
	aboard: boolean;
	saved: boolean;
	// "It's me" jump animation triggered from sidebar click.
	spotlight: number; // > 0 means active (countdown in seconds)
	spotlightPhase: number;
}

interface DisembarkAnim {
	name: string;
	x: number;
	targetX: number;
	groundY: number;
	phase: number;
	waving: boolean;
	waveTimer: number;
	wavePhase: number;
	done: boolean;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

// How fast the lander rotates when an arrow key is held (rad/s).
const ROTATION_SPEED = 3;

// Thrust acceleration magnitude (px/s²).
const THRUST_ACCEL = 50;

// Lunar gravity: constant downward acceleration (px/s²).
const GRAVITY = THRUST_ACCEL / 5;

// How long the explosion animation plays before respawn (seconds).
const EXPLOSION_DURATION = 2.5;

// Number of debris lines in the explosion.
const EXPLOSION_PARTICLES = 28;

// The lander spans from y=-7 (dome) to y=8 (feet) = 15 px tall.
const LANDER_HEIGHT = 15;

// Maximum touchdown speed: one lander height per second.
const LANDING_MAX_SPEED = LANDER_HEIGHT;

// Maximum tilt from vertical for a successful landing (10°).
const LANDING_ANGLE_TOL = (10 * Math.PI) / 180;

// How many Codernauts appear per round.
const CODERNAUTS_PER_ROUND = 10;

// Delay before starting the next round after all are saved (seconds).
const ROUND_TRANSITION_DELAY = 3;

// How long the victory celebration plays (seconds).
const VICTORY_DURATION = 6;

// How long the intro title is shown (seconds).
const INTRO_DURATION = 7;

// Fuel consumption rates.
const FUEL_BURN_MAIN = 2; // % per second of main thruster
const FUEL_BURN_ROT = 0.5; // % per second of rotation thruster
const FUEL_REFILL_RATE = 5; // % per second when landed at the base
const FUEL_WARN_THRESHOLD = 10; // % below which warning is shown

// Lander shape as connected polylines, coordinates relative to centre.
// Roughly 22 px tall before any scaling.
const LANDER_SHAPE: Point[][] = [
	// Body (trapezoid)
	[
		{ x: -5, y: -1 },
		{ x: -7, y: 4 },
		{ x: 7, y: 4 },
		{ x: 5, y: -1 },
		{ x: -5, y: -1 },
	],
	// Dome
	[
		{ x: -4, y: -1 },
		{ x: -3.5, y: -4 },
		{ x: -2, y: -6 },
		{ x: 0, y: -7 },
		{ x: 2, y: -6 },
		{ x: 3.5, y: -4 },
		{ x: 4, y: -1 },
	],
	// Left leg
	[
		{ x: -6, y: 4 },
		{ x: -9, y: 8 },
	],
	// Left foot
	[
		{ x: -11, y: 8 },
		{ x: -7, y: 8 },
	],
	// Right leg
	[
		{ x: 6, y: 4 },
		{ x: 9, y: 8 },
	],
	// Right foot
	[
		{ x: 7, y: 8 },
		{ x: 11, y: 8 },
	],
	// Nozzle
	[
		{ x: -2, y: 4 },
		{ x: -1.5, y: 7 },
		{ x: 1.5, y: 7 },
		{ x: 2, y: 4 },
	],
];

// ---------------------------------------------------------------------------
// Terrain generation
// ---------------------------------------------------------------------------

/**
 * Recursively subdivide a polyline using midpoint displacement to
 * produce organic, jagged terrain.
 */
function subdivide(pts: Point[], roughness: number, depth: number): Point[] {
	if (depth <= 0 || pts.length < 2) return pts;

	const out: Point[] = [];
	for (let i = 0; i < pts.length - 1; i++) {
		const a = pts[i];
		const b = pts[i + 1];
		out.push(a);

		const mx = (a.x + b.x) / 2;
		const my = (a.y + b.y) / 2;
		const disp = (Math.random() - 0.5) * roughness * (b.x - a.x);
		out.push({ x: mx, y: my + disp });
	}
	out.push(pts[pts.length - 1]);

	return subdivide(out, roughness * 0.55, depth - 1);
}

function generateTerrain(w: number, h: number): Terrain {
	// -- landing pads (sorted left to right) --
	const padDefs = [
		{ mult: 2, relW: 0.06 },
		{ mult: 5, relW: 0.025 },
		{ mult: 2, relW: 0.055 },
		{ mult: 3, relW: 0.035 },
	];

	const margin = w * 0.04;
	const usable = w - 2 * margin;
	const section = usable / padDefs.length;

	const pads: LandingPad[] = padDefs
		.map((def, i) => {
			const padW = w * def.relW;
			const cx =
				margin + section * (i + 0.5) + (Math.random() - 0.5) * section * 0.3;
			const cy = h * (0.75 + Math.random() * 0.15);
			return {
				x: cx - padW / 2,
				width: padW,
				y: cy,
				multiplier: def.mult,
				isStation: false,
			};
		})
		.sort((a, b) => a.x - b.x);

	// Nominate the widest pad as the Coder space station.
	let widest = pads[0];
	for (const p of pads) {
		if (p.width > widest.width) widest = p;
	}
	widest.isStation = true;

	// -- control points: peaks / valleys between pads --
	const ctrl: Point[] = [{ x: 0, y: h * (0.65 + Math.random() * 0.2) }];

	for (const pad of pads) {
		const prev = ctrl[ctrl.length - 1];
		const gap = pad.x - prev.x;
		if (gap > w * 0.08) {
			const peakX = prev.x + gap * (0.3 + Math.random() * 0.4);
			// Allow mountains to reach the upper quarter of the screen.
			const peakY = h * (0.25 + Math.random() * 0.5);
			ctrl.push({ x: peakX, y: peakY });
		}
		ctrl.push({ x: pad.x, y: pad.y });
		ctrl.push({ x: pad.x + pad.width, y: pad.y });
	}

	const lastX = ctrl[ctrl.length - 1].x;
	const finalGap = w - lastX;
	if (finalGap > w * 0.08) {
		ctrl.push({
			x: lastX + finalGap * (0.3 + Math.random() * 0.4),
			y: h * (0.3 + Math.random() * 0.45),
		});
	}
	ctrl.push({ x: w, y: h * (0.65 + Math.random() * 0.2) });

	// -- subdivide non-pad segments --
	const points: Point[] = [];
	for (let i = 0; i < ctrl.length - 1; i++) {
		const p1 = ctrl[i];
		const p2 = ctrl[i + 1];

		const isPad = pads.some(
			(pad) =>
				Math.abs(p1.x - pad.x) < 1 &&
				Math.abs(p2.x - (pad.x + pad.width)) < 1 &&
				Math.abs(p1.y - pad.y) < 1,
		);

		if (isPad) {
			points.push(p1);
		} else {
			const seg = subdivide([p1, p2], 0.6, 7);
			for (let j = 0; j < seg.length - 1; j++) {
				points.push(seg[j]);
			}
		}
	}
	points.push(ctrl[ctrl.length - 1]);

	// Clamp heights to stay on screen.
	for (const p of points) {
		p.y = Math.max(h * 0.08, Math.min(h * 0.95, p.y));
	}

	return { points, pads };
}

// ---------------------------------------------------------------------------
// Codernauts - little astronauts stranded on the landing pads
// ---------------------------------------------------------------------------

function shuffle<T>(arr: T[]): T[] {
	const a = [...arr];
	for (let i = a.length - 1; i > 0; i--) {
		const j = Math.floor(Math.random() * (i + 1));
		[a[i], a[j]] = [a[j], a[i]];
	}
	return a;
}

function createCodernauts(
	pads: LandingPad[],
	entries: { name: string; role: string }[],
): Codernaut[] {
	const nauts: Codernaut[] = [];

	// Determine capacity of each non-station pad based on its
	// relative width: wide >= 0.05 -> 5, mid >= 0.03 -> 3-4, small -> 2.
	interface PadSlot {
		pi: number;
		cap: number;
	}
	const slots: PadSlot[] = [];
	for (let pi = 0; pi < pads.length; pi++) {
		const pad = pads[pi];
		if (pad.isStation) continue;
		const rel = pad.width / (window.innerWidth || 1200);
		let cap: number;
		if (rel >= 0.05) cap = 5;
		else if (rel >= 0.03) cap = 3 + Math.round(Math.random());
		else cap = 2;
		slots.push({ pi, cap });
	}
	if (slots.length === 0) return nauts;

	// Distribute entries across pads, filling up to each pad's
	// capacity before moving to the next.
	let ei = 0;
	for (const slot of slots) {
		const pad = pads[slot.pi];
		const inset = pad.width * 0.12;
		let placed = 0;
		while (placed < slot.cap && ei < entries.length) {
			const entry = entries[ei];
			nauts.push({
				x: pad.x + inset + Math.random() * (pad.width - 2 * inset),
				padIdx: slot.pi,
				dir: Math.random() > 0.5 ? 1 : -1,
				speed: 8 + Math.random() * 7,
				walkPhase: Math.random() * Math.PI * 2,
				waving: false,
				waveTimer: 0,
				wavePhase: 0,
				nextWave: 1.5 + Math.random() * 3,
				name: entry.name,
				role: entry.role,
				aboard: false,
				saved: false,
				spotlight: 0,
				spotlightPhase: 0,
			});
			ei++;
			placed++;
		}
	}
	return nauts;
}

function updateCodernauts(nauts: Codernaut[], pads: LandingPad[], dt: number) {
	for (const n of nauts) {
		if (n.aboard || n.saved) continue;

		// Spotlight "It's me" animation.
		if (n.spotlight > 0) {
			n.spotlight -= dt;
			n.spotlightPhase += dt;
			n.wavePhase += dt * 9;
			if (n.spotlight <= 0) {
				n.spotlight = 0;
				n.waving = false;
			}
			continue;
		}

		const pad = pads[n.padIdx];
		const edgeInset = 4;
		const left = pad.x + edgeInset;
		const right = pad.x + pad.width - edgeInset;

		if (n.waving) {
			n.waveTimer -= dt;
			n.wavePhase += dt * 9;
			if (n.waveTimer <= 0) {
				n.waving = false;
				n.nextWave = 2 + Math.random() * 4;
			}
		} else {
			n.x += n.dir * n.speed * dt;
			n.walkPhase += n.speed * dt * 0.6;

			if (n.x >= right) {
				n.x = right;
				n.dir = -1;
			} else if (n.x <= left) {
				n.x = left;
				n.dir = 1;
			}

			n.nextWave -= dt;
			if (n.nextWave <= 0) {
				n.waving = true;
				n.waveTimer = 0.8 + Math.random() * 1.2;
				n.wavePhase = 0;
			}
		}
	}
}

/**
 * Draw a single Codernaut at their position on the pad surface.
 * The figure is ~10 px tall: round helmet, stick body, animated
 * legs and arms. Drawn in gray to match the station aesthetic.
 */
function drawCodernaut(
	ctx: CanvasRenderingContext2D,
	naut: Codernaut,
	groundY: number,
) {
	const color = "#888";

	ctx.save();
	// Jump offset: three bounces over the spotlight duration.
	let jumpY = 0;
	if (naut.spotlight > 0) {
		jumpY = -Math.abs(Math.sin(naut.spotlightPhase * Math.PI * 3)) * 10;
	}
	ctx.translate(naut.x, groundY + jumpY);

	ctx.strokeStyle = color;
	ctx.fillStyle = color;
	ctx.lineWidth = 1.2;
	ctx.lineCap = "round";
	ctx.lineJoin = "round";

	// -- Proportions (measured upward from feet at y = 0) --
	// Small head + thicker body/limbs so the suit reads at game scale.
	const headR = 1.3;
	const headY = -9.2;
	const shoulderY = -7.5;
	const waistY = -3.5;

	// -- Helmet (filled solid gray) --
	ctx.beginPath();
	ctx.arc(0, headY, headR, 0, Math.PI * 2);
	ctx.fill();
	ctx.stroke();

	// Visor: a darker slit across the face.
	ctx.strokeStyle = "#555";
	ctx.lineWidth = 1.4;
	ctx.beginPath();
	ctx.moveTo(-headR * 0.55, headY);
	ctx.lineTo(headR * 0.55, headY);
	ctx.stroke();
	ctx.strokeStyle = color;

	// -- Body (thicker line for the suit) --
	ctx.lineWidth = 2;
	ctx.beginPath();
	ctx.moveTo(0, headY + headR);
	ctx.lineTo(0, waistY);
	ctx.stroke();

	// -- Legs (thicker) --
	ctx.lineWidth = 1.6;
	const legLen = 3.5;
	if (naut.waving) {
		// Standing still: legs apart.
		ctx.beginPath();
		ctx.moveTo(0, waistY);
		ctx.lineTo(-2, 0);
		ctx.moveTo(0, waistY);
		ctx.lineTo(2, 0);
		ctx.stroke();
	} else {
		const swing = Math.sin(naut.walkPhase) * legLen * 0.4;
		ctx.beginPath();
		ctx.moveTo(0, waistY);
		ctx.lineTo(swing, 0);
		ctx.moveTo(0, waistY);
		ctx.lineTo(-swing, 0);
		ctx.stroke();
	}

	// -- Arms (thicker) --
	const armLen = 3.8;
	if (naut.waving) {
		// Both arms wave diagonally outward, well clear of the
		// head, for a frantic "help me!" gesture. The base angle
		// is ~60° from vertical so the hands stay to the sides.
		const wave1 = Math.sin(naut.wavePhase) * 0.35;
		const wave2 = Math.sin(naut.wavePhase + 1.8) * 0.35;
		const base1 = -Math.PI / 4 + wave1;
		const base2 = -Math.PI / 4 + wave2;

		// Right arm.
		ctx.beginPath();
		ctx.moveTo(0, shoulderY);
		ctx.lineTo(Math.cos(base1) * armLen, shoulderY + Math.sin(base1) * armLen);
		ctx.stroke();

		// Left arm (mirrored).
		ctx.beginPath();
		ctx.moveTo(0, shoulderY);
		ctx.lineTo(-Math.cos(base2) * armLen, shoulderY + Math.sin(base2) * armLen);
		ctx.stroke();
	} else {
		// Arms swing opposite to legs.
		const armSwing = Math.sin(naut.walkPhase + Math.PI * 0.5) * 0.4;
		const ax = Math.sin(armSwing) * armLen;
		const ay = Math.cos(armSwing) * armLen * 0.8;
		ctx.beginPath();
		ctx.moveTo(0, shoulderY);
		ctx.lineTo(ax, shoulderY + ay);
		ctx.moveTo(0, shoulderY);
		ctx.lineTo(-ax, shoulderY + ay);
		ctx.stroke();
	}

	ctx.restore();
}

// ---------------------------------------------------------------------------
// Drawing helpers
// ---------------------------------------------------------------------------

function drawTerrain(ctx: CanvasRenderingContext2D, terrain: Terrain) {
	const { points, pads } = terrain;

	// Terrain outline.
	ctx.strokeStyle = "white";
	ctx.lineWidth = 1;
	ctx.beginPath();
	if (points.length > 0) {
		ctx.moveTo(points[0].x, points[0].y);
		for (let i = 1; i < points.length; i++) {
			ctx.lineTo(points[i].x, points[i].y);
		}
	}
	ctx.stroke();

	// Landing-pad markers and labels.
	const tick = 5;
	for (const pad of pads) {
		const left = pad.x;
		const right = pad.x + pad.width;
		const midX = left + pad.width / 2;

		// Small vertical ticks at each edge.
		ctx.strokeStyle = "white";
		ctx.lineWidth = 1;
		ctx.beginPath();
		ctx.moveTo(left, pad.y);
		ctx.lineTo(left, pad.y + tick);
		ctx.moveTo(right, pad.y);
		ctx.lineTo(right, pad.y + tick);
		ctx.stroke();

		if (pad.isStation) {
			drawSpaceStation(ctx, midX, pad.y, pad.width);
			drawVectorText(ctx, "CODER BASE", midX, pad.y + tick + 10, 7);
		} else {
			const helpH = Math.min(10, pad.width / 4);
			drawVectorText(ctx, "HELP", midX, pad.y + tick + 10, helpH);
		}
	}
}

/**
 * Draw a symbolic moon space station centred at (cx, groundY) sitting
 * on the landing pad. All strokes are gray so the station is visually
 * distinct from the white terrain and lander.
 */
function drawSpaceStation(
	ctx: CanvasRenderingContext2D,
	cx: number,
	groundY: number,
	padWidth: number,
) {
	// Scale the station to roughly 60% of the pad width.
	const s = padWidth * 0.6;
	const color = "#888";

	ctx.save();
	ctx.strokeStyle = color;
	ctx.fillStyle = color;
	ctx.lineWidth = 1.5;
	ctx.lineCap = "round";
	ctx.lineJoin = "round";

	// -- main habitat module (rectangle sitting on the pad) --
	const habW = s * 0.55;
	const habH = s * 0.3;
	const habTop = groundY - habH;
	ctx.strokeRect(cx - habW / 2, habTop, habW, habH);

	// -- dome on top of the habitat --
	const domeR = habW * 0.35;
	ctx.beginPath();
	ctx.arc(cx, habTop, domeR, Math.PI, 0);
	ctx.stroke();

	// -- antenna mast rising from the dome --
	const mastTop = habTop - domeR - s * 0.25;
	ctx.beginPath();
	ctx.moveTo(cx, habTop - domeR);
	ctx.lineTo(cx, mastTop);
	ctx.stroke();

	// -- small dish at the top of the antenna --
	const dishW = s * 0.12;
	ctx.beginPath();
	ctx.moveTo(cx - dishW, mastTop + s * 0.04);
	ctx.lineTo(cx, mastTop);
	ctx.lineTo(cx + dishW, mastTop + s * 0.04);
	ctx.stroke();

	// -- solar panels on each side --
	const panelW = s * 0.28;
	const panelH = s * 0.12;
	const panelY = habTop + habH * 0.35;

	// Left panel.
	const lpx = cx - habW / 2 - panelW;
	ctx.strokeRect(lpx, panelY, panelW, panelH);
	// Strut connecting panel to habitat.
	ctx.beginPath();
	ctx.moveTo(cx - habW / 2, panelY + panelH / 2);
	ctx.lineTo(lpx + panelW, panelY + panelH / 2);
	ctx.stroke();

	// Right panel.
	const rpx = cx + habW / 2;
	ctx.strokeRect(rpx, panelY, panelW, panelH);
	ctx.beginPath();
	ctx.moveTo(cx + habW / 2, panelY + panelH / 2);
	ctx.lineTo(rpx, panelY + panelH / 2);
	ctx.stroke();

	// -- small window details on the habitat --
	const winSize = s * 0.06;
	const winY = habTop + habH * 0.4;
	ctx.fillRect(cx - habW * 0.22, winY, winSize, winSize);
	ctx.fillRect(cx + habW * 0.14, winY, winSize, winSize);

	ctx.restore();
}

function drawLander(
	ctx: CanvasRenderingContext2D,
	x: number,
	y: number,
	angle: number,
	thrusting: boolean,
) {
	ctx.save();
	ctx.translate(x, y);
	ctx.rotate(angle);

	ctx.strokeStyle = "white";
	ctx.lineWidth = 1.5;
	ctx.lineCap = "round";
	ctx.lineJoin = "round";

	for (const path of LANDER_SHAPE) {
		ctx.beginPath();
		ctx.moveTo(path[0].x, path[0].y);
		for (let i = 1; i < path.length; i++) {
			ctx.lineTo(path[i].x, path[i].y);
		}
		ctx.stroke();
	}

	// Animated rocket flame emerging from the nozzle when thrusting.
	if (thrusting) {
		const flameLen = 8 + Math.random() * 10;
		const flameW = 2.2 + Math.random() * 1.5;

		// Outer flame cone.
		ctx.beginPath();
		ctx.moveTo(-flameW, 7);
		ctx.lineTo(0, 7 + flameLen);
		ctx.lineTo(flameW, 7);
		ctx.stroke();

		// Inner flame (shorter, narrower) for depth.
		const innerLen = flameLen * (0.4 + Math.random() * 0.25);
		const innerW = flameW * 0.45;
		ctx.beginPath();
		ctx.moveTo(-innerW, 7);
		ctx.lineTo(0, 7 + innerLen);
		ctx.lineTo(innerW, 7);
		ctx.stroke();
	}

	ctx.restore();
}

// ---------------------------------------------------------------------------
// Dashboard - instrument panels at the bottom of the screen
// ---------------------------------------------------------------------------

// Height reserved for the dashboard strip.
const DASHBOARD_H = 84;

// Width reserved for the roster sidebar on the right.
const SIDEBAR_W = 200;

interface DashboardState {
	velocity: number;
	yaw: number;
	altitude: number;
	fuel: number;
}

// --- 7-segment digit renderer -----------------------------------------------
//
// Segments labelled a-g in the standard layout:
//   aaa
//  f   b
//   ggg
//  e   c
//   ddd
//
// Each segment is a short line drawn at the appropriate position
// within a character cell of size (cw x ch).

const SEG_PATTERNS: Record<string, string> = {
	"0": "abcdef",
	"1": "bc",
	"2": "abdeg",
	"3": "abcdg",
	"4": "bcfg",
	"5": "acdfg",
	"6": "acdefg",
	"7": "abc",
	"8": "abcdefg",
	"9": "abcdfg",
	"-": "g",
	" ": "",
};

function draw7Seg(
	ctx: CanvasRenderingContext2D,
	text: string,
	startX: number,
	y: number,
	cw: number,
	ch: number,
) {
	const m = 1; // inset so segments don’t touch corners
	const halfH = ch / 2;

	for (let i = 0; i < text.length; i++) {
		const x = startX + i * (cw + 3);
		const char = text[i];

		// Decimal point.
		if (char === ".") {
			ctx.beginPath();
			ctx.arc(x + cw * 0.3, y + ch, 1.2, 0, Math.PI * 2);
			ctx.fill();
			continue;
		}

		// Degree symbol.
		if (char === "\u00b0") {
			ctx.beginPath();
			ctx.arc(x + cw * 0.4, y + 1, 2.5, 0, Math.PI * 2);
			ctx.stroke();
			continue;
		}

		// Percent - draw as small slash with two dots.
		if (char === "%") {
			ctx.beginPath();
			ctx.arc(x + 2, y + 3, 1.5, 0, Math.PI * 2);
			ctx.fill();
			ctx.beginPath();
			ctx.moveTo(x + cw - 1, y + 1);
			ctx.lineTo(x + 1, y + ch - 1);
			ctx.stroke();
			ctx.beginPath();
			ctx.arc(x + cw - 2, y + ch - 3, 1.5, 0, Math.PI * 2);
			ctx.fill();
			continue;
		}

		const segs = SEG_PATTERNS[char];
		if (segs === undefined) continue;

		ctx.beginPath();
		for (const s of segs) {
			switch (s) {
				case "a":
					ctx.moveTo(x + m, y);
					ctx.lineTo(x + cw - m, y);
					break;
				case "b":
					ctx.moveTo(x + cw, y + m);
					ctx.lineTo(x + cw, y + halfH - m);
					break;
				case "c":
					ctx.moveTo(x + cw, y + halfH + m);
					ctx.lineTo(x + cw, y + ch - m);
					break;
				case "d":
					ctx.moveTo(x + m, y + ch);
					ctx.lineTo(x + cw - m, y + ch);
					break;
				case "e":
					ctx.moveTo(x, y + halfH + m);
					ctx.lineTo(x, y + ch - m);
					break;
				case "f":
					ctx.moveTo(x, y + m);
					ctx.lineTo(x, y + halfH - m);
					break;
				case "g":
					ctx.moveTo(x + m, y + halfH);
					ctx.lineTo(x + cw - m, y + halfH);
					break;
			}
		}
		ctx.stroke();
	}
}

/** Measure how many pixels wide a draw7Seg string will be. */
function seg7Width(text: string, cw: number): number {
	if (text.length === 0) return 0;
	return text.length * (cw + 3) - 3;
}

// --- dashboard drawing ------------------------------------------------------

/**
 * Draw the control dashboard below the playfield. Four small
 * instrument panels on the left, one wide cargo manifest on the
 * right, all in monochrome vector style.
 */
function drawDashboard(
	ctx: CanvasRenderingContext2D,
	w: number,
	h: number,
	state: DashboardState,
	cargo: Codernaut[],
	savedPct: number,
) {
	const top = h - DASHBOARD_H;

	// Background strip.
	ctx.fillStyle = "#111";
	ctx.fillRect(0, top, w, DASHBOARD_H);

	// Divider line between playfield and dashboard.
	ctx.strokeStyle = "#444";
	ctx.lineWidth = 1;
	ctx.beginPath();
	ctx.moveTo(0, top);
	ctx.lineTo(w, top);
	ctx.stroke();

	// --- layout: 5 small panels + 1 wide cargo panel ---
	// Proportions: each small panel = 1 unit, cargo = 2 units.
	const padX = 10;
	const gap = 8;
	const units = 7; // 5×1 + 1×2
	const totalGap = padX * 2 + gap * 5; // 6 panels → 5 gaps
	const unitW = (w - totalGap) / units;
	const smallW = unitW;
	const cargoW = unitW * 2;
	const panelH = DASHBOARD_H - 16;
	const panelY = top + 8;

	// Helper: draw a single bezel panel and return its x.
	function drawBezel(px: number, pw: number) {
		const r = 5;
		ctx.strokeStyle = "#555";
		ctx.lineWidth = 1.5;
		ctx.beginPath();
		ctx.moveTo(px + r, panelY);
		ctx.lineTo(px + pw - r, panelY);
		ctx.arcTo(px + pw, panelY, px + pw, panelY + r, r);
		ctx.lineTo(px + pw, panelY + panelH - r);
		ctx.arcTo(px + pw, panelY + panelH, px + pw - r, panelY + panelH, r);
		ctx.lineTo(px + r, panelY + panelH);
		ctx.arcTo(px, panelY + panelH, px, panelY + panelH - r, r);
		ctx.lineTo(px, panelY + r);
		ctx.arcTo(px, panelY, px + r, panelY, r);
		ctx.closePath();
		ctx.fillStyle = "#0a0a0a";
		ctx.fill();
		ctx.stroke();

		// Corner screws.
		ctx.fillStyle = "#333";
		for (const [sx, sy] of [
			[px + 5, panelY + 5],
			[px + pw - 5, panelY + 5],
			[px + 5, panelY + panelH - 5],
			[px + pw - 5, panelY + panelH - 5],
		]) {
			ctx.beginPath();
			ctx.arc(sx, sy, 2, 0, Math.PI * 2);
			ctx.fill();
		}
	}

	// Helper: draw panel label.
	function drawLabel(px: number, pw: number, label: string) {
		ctx.fillStyle = "#666";
		ctx.font = "bold 9px monospace";
		ctx.textAlign = "center";
		ctx.fillText(label, px + pw / 2, panelY + 14);
	}

	// Helper: draw 7-segment readout centred in a panel.
	function drawReadout(px: number, pw: number, text: string) {
		const cw = 7;
		const ch = 12;
		const tw = seg7Width(text, cw);
		const rx = px + (pw - tw) / 2;
		const ry = panelY + 22;

		ctx.strokeStyle = "white";
		ctx.fillStyle = "white";
		ctx.lineWidth = 1.6;
		ctx.lineCap = "round";
		draw7Seg(ctx, text, rx, ry, cw, ch);
	}

	// --- small panels (velocity, yaw, altitude, fuel) ---
	const yawDeg = (state.yaw * 180) / Math.PI;
	const velWarn = state.velocity > LANDING_MAX_SPEED;
	const yawWarn = Math.abs(yawDeg) > (LANDING_ANGLE_TOL * 180) / Math.PI;

	const smallPanels = [
		{ label: "VELOCITY", value: `${state.velocity.toFixed(1)}`, warn: velWarn },
		{ label: "YAW", value: `${yawDeg.toFixed(1)}\u00b0`, warn: yawWarn },
		{
			label: "ALTITUDE",
			value: `${Math.max(0, state.altitude).toFixed(0)}`,
			warn: false,
		},
		{
			label: "FUEL",
			value: `${state.fuel.toFixed(0)}%`,
			warn: state.fuel < FUEL_WARN_THRESHOLD,
		},
		{ label: "SAVED", value: `${savedPct.toFixed(0)}%`, warn: false },
	];

	let cx = padX;
	for (const sp of smallPanels) {
		drawBezel(cx, smallW);
		drawLabel(cx, smallW, sp.label);
		drawReadout(cx, smallW, sp.value);
		if (sp.warn) {
			ctx.fillStyle = "#fff";
			ctx.font = "bold 8px monospace";
			ctx.textAlign = "center";
			ctx.fillText("⚠ WARNING", cx + smallW / 2, panelY + panelH - 7);
		}
		cx += smallW + gap;
	}

	// --- cargo panel (wide) ---
	drawBezel(cx, cargoW);
	drawLabel(cx, cargoW, "CARGO");

	ctx.font = "9px monospace";
	ctx.textAlign = "left";
	for (let i = 0; i < 4; i++) {
		const sx = cx + 12;
		const sy = panelY + 24 + i * 11;
		if (i < cargo.length) {
			ctx.fillStyle = "white";
			ctx.fillText(`${i + 1}: ${cargo[i].name} - ${cargo[i].role}`, sx, sy);
		} else {
			ctx.fillStyle = "#555";
			ctx.fillText(`${i + 1}: ---- unoccupied ----`, sx, sy);
		}
	}
}

// ---------------------------------------------------------------------------
// Collision detection & explosions
// ---------------------------------------------------------------------------

/**
 * Return the terrain surface height (y) at the given x by linearly
 * interpolating between the two nearest terrain points.
 */
function terrainHeightAt(x: number, points: Point[]): number {
	if (points.length === 0) return 1e9;
	if (x <= points[0].x) return points[0].y;
	if (x >= points[points.length - 1].x) return points[points.length - 1].y;

	for (let i = 0; i < points.length - 1; i++) {
		const a = points[i];
		const b = points[i + 1];
		if (x >= a.x && x <= b.x) {
			const t = (x - a.x) / (b.x - a.x);
			return a.y + t * (b.y - a.y);
		}
	}
	return points[points.length - 1].y;
}

/**
 * Check whether the lander (centred at x, y) has hit the terrain.
 * We test the bottom of the lander (y + 8 in local coords at
 * angle 0) against the surface height.
 */
function landerHitsTerrain(
	x: number,
	y: number,
	angle: number,
	terrainPts: Point[],
): boolean {
	// Sample several points around the lander footprint.
	const offsets = [
		{ lx: 0, ly: 8 }, // nozzle base / feet
		{ lx: -11, ly: 8 }, // left foot
		{ lx: 11, ly: 8 }, // right foot
		{ lx: 0, ly: -7 }, // dome top
	];
	const cosA = Math.cos(angle);
	const sinA = Math.sin(angle);
	for (const o of offsets) {
		const wx = x + o.lx * cosA - o.ly * sinA;
		const wy = y + o.lx * sinA + o.ly * cosA;
		const surfaceY = terrainHeightAt(wx, terrainPts);
		if (wy >= surfaceY) return true;
	}
	return false;
}

function createExplosion(x: number, y: number): ExplosionParticle[] {
	const parts: ExplosionParticle[] = [];
	for (let i = 0; i < EXPLOSION_PARTICLES; i++) {
		const a = Math.random() * Math.PI * 2;
		// Mix of fast shrapnel and slower drifting debris.
		const fast = i < EXPLOSION_PARTICLES * 0.4;
		const speed = fast ? 80 + Math.random() * 140 : 20 + Math.random() * 60;
		parts.push({
			x,
			y,
			vx: Math.cos(a) * speed,
			vy: Math.sin(a) * speed - (fast ? 40 : 10),
			len: fast ? 5 + Math.random() * 10 : 2 + Math.random() * 5,
			angle: Math.random() * Math.PI * 2,
			spin: (Math.random() - 0.5) * 14,
		});
	}
	return parts;
}

function updateExplosion(parts: ExplosionParticle[], dt: number) {
	for (const p of parts) {
		p.vy += GRAVITY * 1.5 * dt;
		p.x += p.vx * dt;
		p.y += p.vy * dt;
		p.angle += p.spin * dt;
		p.vx *= 1 - 0.35 * dt;
		p.vy *= 1 - 0.35 * dt;
	}
}

function drawExplosion(
	ctx: CanvasRenderingContext2D,
	parts: ExplosionParticle[],
	progress: number,
) {
	ctx.save();
	ctx.lineCap = "round";

	// Brief bright flash at the start of the explosion.
	if (progress > 0.85) {
		const flash = (progress - 0.85) / 0.15;
		ctx.fillStyle = `rgba(255,255,255,${(flash * 0.35).toFixed(2)})`;
		ctx.beginPath();
		ctx.arc(
			parts[0]?.x ?? 0,
			parts[0]?.y ?? 0,
			25 + (1 - flash) * 30,
			0,
			Math.PI * 2,
		);
		ctx.fill();
	}

	for (const p of parts) {
		// Each particle fades based on overall progress.
		const a = Math.max(0, progress * (0.6 + Math.random() * 0.4));
		ctx.strokeStyle = `rgba(255,255,255,${a.toFixed(2)})`;
		ctx.lineWidth = 1 + progress;
		const dx = Math.cos(p.angle) * p.len * 0.5;
		const dy = Math.sin(p.angle) * p.len * 0.5;
		ctx.beginPath();
		ctx.moveTo(p.x - dx, p.y - dy);
		ctx.lineTo(p.x + dx, p.y + dy);
		ctx.stroke();
	}

	ctx.restore();
}

// ---------------------------------------------------------------------------
// Landing helpers
// ---------------------------------------------------------------------------

/** Normalise an angle into the [−π, π) range. */
function normalizeAngle(a: number): number {
	let n = a % (Math.PI * 2);
	if (n >= Math.PI) n -= Math.PI * 2;
	if (n < -Math.PI) n += Math.PI * 2;
	return n;
}

/**
 * Return the landing pad whose flat surface the lander’s centre-x
 * sits over, or null if the lander is not above any pad.
 */
function findPadAt(x: number, pads: LandingPad[]): LandingPad | null {
	for (const pad of pads) {
		if (x >= pad.x && x <= pad.x + pad.width) return pad;
	}
	return null;
}

/**
 * Determine whether the lander qualifies for a safe touchdown.
 * Returns the pad if successful, null otherwise (= crash).
 */
function tryLanding(
	x: number,
	_y: number,
	angle: number,
	vx: number,
	vy: number,
	pads: LandingPad[],
): LandingPad | null {
	const pad = findPadAt(x, pads);
	if (!pad) return null;

	// Orientation: must be roughly upright.
	if (Math.abs(normalizeAngle(angle)) > LANDING_ANGLE_TOL) return null;

	// Speed: magnitude must be within tolerance.
	const speed = Math.sqrt(vx * vx + vy * vy);
	if (speed > LANDING_MAX_SPEED) return null;

	return pad;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Disembark animation - Codernauts walking into the base
// ---------------------------------------------------------------------------

function updateDisembarks(anims: DisembarkAnim[], dt: number) {
	for (const a of anims) {
		if (a.done) continue;
		if (a.waving) {
			a.wavePhase += dt * 9;
			a.waveTimer -= dt;
			if (a.waveTimer <= 0) a.done = true;
		} else {
			// Walk toward the target.
			const dx = a.targetX - a.x;
			const step = 30 * dt;
			if (Math.abs(dx) < step) {
				a.x = a.targetX;
				a.waving = true;
				a.waveTimer = 1.2;
				a.wavePhase = 0;
			} else {
				a.x += Math.sign(dx) * step;
			}
			a.phase += 15 * dt;
		}
	}
}

// ---------------------------------------------------------------------------
// Tooltip - comic-style speech bubble on hover
// ---------------------------------------------------------------------------

function drawTooltip(
	ctx: CanvasRenderingContext2D,
	name: string,
	role: string,
	tipX: number,
	tipY: number,
) {
	ctx.save();
	ctx.font = "bold 11px monospace";
	const nameW = ctx.measureText(name).width;
	ctx.font = "10px monospace";
	const roleW = ctx.measureText(role).width;

	const boxW = Math.max(nameW, roleW) + 16;
	const boxH = 34;
	const tailH = 8;
	const bx = tipX - boxW / 2;
	const by = tipY - boxH - tailH - 4;
	const r = 6;

	// Bubble background.
	ctx.fillStyle = "#222";
	ctx.strokeStyle = "#888";
	ctx.lineWidth = 1.5;
	ctx.beginPath();
	ctx.moveTo(bx + r, by);
	ctx.lineTo(bx + boxW - r, by);
	ctx.arcTo(bx + boxW, by, bx + boxW, by + r, r);
	ctx.lineTo(bx + boxW, by + boxH - r);
	ctx.arcTo(bx + boxW, by + boxH, bx + boxW - r, by + boxH, r);
	// Tail.
	ctx.lineTo(tipX + 6, by + boxH);
	ctx.lineTo(tipX, by + boxH + tailH);
	ctx.lineTo(tipX - 6, by + boxH);
	ctx.lineTo(bx + r, by + boxH);
	ctx.arcTo(bx, by + boxH, bx, by + boxH - r, r);
	ctx.lineTo(bx, by + r);
	ctx.arcTo(bx, by, bx + r, by, r);
	ctx.closePath();
	ctx.fill();
	ctx.stroke();

	// Name.
	ctx.fillStyle = "white";
	ctx.font = "bold 11px monospace";
	ctx.textAlign = "center";
	ctx.fillText(name, bx + boxW / 2, by + 14);

	// Role.
	ctx.fillStyle = "#aaa";
	ctx.font = "10px monospace";
	ctx.fillText(role, bx + boxW / 2, by + 27);

	ctx.restore();
}

// ---------------------------------------------------------------------------
// Vector text renderer
// ---------------------------------------------------------------------------

// Simple vector font: each letter is an array of polylines
// defined in a 0-1 normalised coordinate space (width x height).
const VECTOR_FONT: Record<string, number[][][]> = {
	A: [
		[
			[0, 1],
			[0.5, 0],
			[1, 1],
		],
		[
			[0.15, 0.65],
			[0.85, 0.65],
		],
	],
	B: [
		[
			[0, 1],
			[0, 0],
			[0.7, 0],
			[0.9, 0.15],
			[0.9, 0.35],
			[0.7, 0.5],
			[0, 0.5],
		],
		[
			[0.7, 0.5],
			[0.9, 0.65],
			[0.9, 0.85],
			[0.7, 1],
			[0, 1],
		],
	],
	C: [
		[
			[1, 0.15],
			[0.7, 0],
			[0.3, 0],
			[0, 0.15],
			[0, 0.85],
			[0.3, 1],
			[0.7, 1],
			[1, 0.85],
		],
	],
	D: [
		[
			[0, 0],
			[0, 1],
			[0.6, 1],
			[0.9, 0.8],
			[0.9, 0.2],
			[0.6, 0],
			[0, 0],
		],
	],
	E: [
		[
			[1, 0],
			[0, 0],
			[0, 0.5],
			[0.7, 0.5],
		],
		[
			[0, 0.5],
			[0, 1],
			[1, 1],
		],
	],
	F: [
		[
			[1, 0],
			[0, 0],
			[0, 0.5],
			[0.7, 0.5],
		],
		[
			[0, 0.5],
			[0, 1],
		],
	],
	G: [
		[
			[1, 0.15],
			[0.7, 0],
			[0.3, 0],
			[0, 0.15],
			[0, 0.85],
			[0.3, 1],
			[0.7, 1],
			[1, 0.85],
			[1, 0.5],
			[0.55, 0.5],
		],
	],
	H: [
		[
			[0, 0],
			[0, 1],
		],
		[
			[1, 0],
			[1, 1],
		],
		[
			[0, 0.5],
			[1, 0.5],
		],
	],
	I: [
		[
			[0.2, 0],
			[0.8, 0],
		],
		[
			[0.5, 0],
			[0.5, 1],
		],
		[
			[0.2, 1],
			[0.8, 1],
		],
	],
	J: [
		[
			[0.2, 0],
			[0.8, 0],
		],
		[
			[0.5, 0],
			[0.5, 0.85],
			[0.3, 1],
			[0.1, 0.85],
		],
	],
	K: [
		[
			[0, 0],
			[0, 1],
		],
		[
			[1, 0],
			[0, 0.5],
			[1, 1],
		],
	],
	L: [
		[
			[0, 0],
			[0, 1],
			[1, 1],
		],
	],
	M: [
		[
			[0, 1],
			[0, 0],
			[0.5, 0.4],
			[1, 0],
			[1, 1],
		],
	],
	N: [
		[
			[0, 1],
			[0, 0],
			[1, 1],
			[1, 0],
		],
	],
	O: [
		[
			[0.3, 0],
			[0.7, 0],
			[1, 0.15],
			[1, 0.85],
			[0.7, 1],
			[0.3, 1],
			[0, 0.85],
			[0, 0.15],
			[0.3, 0],
		],
	],
	P: [
		[
			[0, 1],
			[0, 0],
			[0.7, 0],
			[1, 0.15],
			[1, 0.35],
			[0.7, 0.5],
			[0, 0.5],
		],
	],
	Q: [
		[
			[0.3, 0],
			[0.7, 0],
			[1, 0.15],
			[1, 0.85],
			[0.7, 1],
			[0.3, 1],
			[0, 0.85],
			[0, 0.15],
			[0.3, 0],
		],
		[
			[0.65, 0.75],
			[1, 1],
		],
	],
	R: [
		[
			[0, 1],
			[0, 0],
			[0.7, 0],
			[1, 0.15],
			[1, 0.35],
			[0.7, 0.5],
			[0, 0.5],
		],
		[
			[0.55, 0.5],
			[1, 1],
		],
	],
	S: [
		[
			[1, 0.15],
			[0.7, 0],
			[0.3, 0],
			[0, 0.15],
			[0, 0.35],
			[0.3, 0.5],
			[0.7, 0.5],
			[1, 0.65],
			[1, 0.85],
			[0.7, 1],
			[0.3, 1],
			[0, 0.85],
		],
	],
	T: [
		[
			[0, 0],
			[1, 0],
		],
		[
			[0.5, 0],
			[0.5, 1],
		],
	],
	U: [
		[
			[0, 0],
			[0, 0.85],
			[0.3, 1],
			[0.7, 1],
			[1, 0.85],
			[1, 0],
		],
	],
	V: [
		[
			[0, 0],
			[0.5, 1],
			[1, 0],
		],
	],
	W: [
		[
			[0, 0],
			[0.25, 1],
			[0.5, 0.5],
			[0.75, 1],
			[1, 0],
		],
	],
	X: [
		[
			[0, 0],
			[1, 1],
		],
		[
			[1, 0],
			[0, 1],
		],
	],
	Y: [
		[
			[0, 0],
			[0.5, 0.5],
			[1, 0],
		],
		[
			[0.5, 0.5],
			[0.5, 1],
		],
	],
	Z: [
		[
			[0, 0],
			[1, 0],
			[0, 1],
			[1, 1],
		],
	],
	"!": [
		[
			[0.5, 0],
			[0.5, 0.65],
		],
		[
			[0.5, 0.8],
			[0.5, 0.85],
		],
	],
	" ": [],
};

/**
 * Draw a string using the vector font, centred horizontally at
 * (centerX, centerY). `lh` is the letter height in pixels.
 */
function drawVectorText(
	ctx: CanvasRenderingContext2D,
	text: string,
	centerX: number,
	centerY: number,
	lh: number,
) {
	const lw = lh * 0.6;
	const spacing = lh * 0.15;
	const totalW = text.length * lw + (text.length - 1) * spacing;
	let x = centerX - totalW / 2;
	const y = centerY - lh / 2;

	ctx.save();
	ctx.strokeStyle = "white";
	ctx.lineWidth = Math.max(1.5, lh / 18);
	ctx.lineCap = "round";
	ctx.lineJoin = "round";

	for (const ch of text) {
		const glyph = VECTOR_FONT[ch];
		if (glyph) {
			for (const poly of glyph) {
				ctx.beginPath();
				for (let i = 0; i < poly.length; i++) {
					const px = x + poly[i][0] * lw;
					const py = y + poly[i][1] * lh;
					if (i === 0) ctx.moveTo(px, py);
					else ctx.lineTo(px, py);
				}
				ctx.stroke();
			}
		}
		x += lw + spacing;
	}

	ctx.restore();
}

// ---------------------------------------------------------------------------
// Victory celebration
// ---------------------------------------------------------------------------

/** Draw large vector-style "TADA!" text centred on screen. */
function drawTada(
	ctx: CanvasRenderingContext2D,
	centerX: number,
	centerY: number,
) {
	drawVectorText(ctx, "TADA!", centerX, centerY, 52);
}

function createCelebrationExplosions(
	w: number,
	h: number,
): ExplosionParticle[][] {
	const bursts: ExplosionParticle[][] = [];
	for (let i = 0; i < 8; i++) {
		const ex = w * 0.1 + Math.random() * w * 0.8;
		const ey = h * 0.15 + Math.random() * h * 0.5;
		bursts.push(createExplosion(ex, ey));
	}
	return bursts;
}

// ---------------------------------------------------------------------------
// Roster sidebar
// ---------------------------------------------------------------------------

/** Shorten text to fit within maxW pixels, appending "..." when needed. */
function truncateText(
	ctx: CanvasRenderingContext2D,
	text: string,
	maxW: number,
): string {
	if (ctx.measureText(text).width <= maxW) return text;
	const ellipsis = "\u2026";
	for (let i = text.length - 1; i > 0; i--) {
		const candidate = text.slice(0, i) + ellipsis;
		if (ctx.measureText(candidate).width <= maxW) return candidate;
	}
	return ellipsis;
}

interface SidebarState {
	scrollOffset: number;
}

function drawSidebar(
	ctx: CanvasRenderingContext2D,
	w: number,
	h: number,
	savedNames: Set<string>,
	currentNames: Set<string>,
	sidebarState: SidebarState,
) {
	const x = w - SIDEBAR_W;
	const rowH = 16;
	const headerH = 24;
	const padX = 8;

	// Background.
	ctx.fillStyle = "#0a0a0a";
	ctx.fillRect(x, 0, SIDEBAR_W, h - DASHBOARD_H);

	// Separator line.
	ctx.strokeStyle = "#444";
	ctx.lineWidth = 1;
	ctx.beginPath();
	ctx.moveTo(x, 0);
	ctx.lineTo(x, h - DASHBOARD_H);
	ctx.stroke();

	// Header.
	ctx.fillStyle = "#666";
	ctx.font = "bold 10px monospace";
	ctx.textAlign = "left";
	ctx.fillText("CODERNAUTS ROSTER", x + padX, headerH - 6);

	// Clip to sidebar area below header.
	ctx.save();
	ctx.beginPath();
	ctx.rect(x, headerH, SIDEBAR_W, h - DASHBOARD_H - headerH);
	ctx.clip();

	ctx.font = "9px monospace";
	let ry = headerH + 4 - sidebarState.scrollOffset;

	for (const entry of ROSTER) {
		if (ry + rowH > headerH && ry < h - DASHBOARD_H) {
			let status: string;
			let color: string;
			if (savedNames.has(entry.name)) {
				status = "\u2713 saved";
				color = "#666";
			} else if (currentNames.has(entry.name)) {
				status = "\u26a0 requesting help";
				color = "white";
			} else {
				status = "\u2026 another base";
				color = "#444";
			}

			const maxTextW = SIDEBAR_W - padX * 2 - 6;
			ctx.fillStyle = color;
			ctx.font = "9px monospace";
			ctx.fillText(truncateText(ctx, entry.name, maxTextW), x + padX, ry + 10);
			ctx.fillStyle = color === "white" ? "#888" : color;
			ctx.fillText(
				truncateText(ctx, `${entry.role} \u2014 ${status}`, maxTextW - 10),
				x + padX + 10,
				ry + 10 + rowH * 0.65,
			);
		}
		ry += rowH * 1.6;
	}

	ctx.restore();

	// Scroll indicators.
	const totalH = ROSTER.length * rowH * 1.6;
	const viewH = h - DASHBOARD_H - headerH;
	if (totalH > viewH) {
		const barH = Math.max(12, (viewH / totalH) * viewH);
		const barY =
			headerH + (sidebarState.scrollOffset / (totalH - viewH)) * (viewH - barH);
		ctx.fillStyle = "#333";
		ctx.fillRect(x + SIDEBAR_W - 4, barY, 3, barH);
	}
}

/**
 * Determine which sidebar row was clicked and return the roster
 * entry name, or null if the click was outside the sidebar.
 */
function sidebarHitTest(
	mx: number,
	my: number,
	w: number,
	h: number,
	scrollOffset: number,
): string | null {
	const x = w - SIDEBAR_W;
	if (mx < x || mx > w) return null;
	if (my < 24 || my > h - DASHBOARD_H) return null;

	const rowH = 16 * 1.6;
	const idx = Math.floor((my - 24 + scrollOffset) / rowH);
	if (idx >= 0 && idx < ROSTER.length) return ROSTER[idx].name;
	return null;
}

export const LunarLander: FC = () => {
	const canvasRef = useRef<HTMLCanvasElement>(null);
	const containerRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const canvas = canvasRef.current;
		const container = containerRef.current;
		if (!canvas || !container) return;

		const ctx = canvas.getContext("2d");
		if (!ctx) return;

		const cvs = canvas;
		const ctr = container;
		const cx = ctx;

		// ---- mutable game state ----
		let terrain: Terrain | null = null;
		let codernauts: Codernaut[] = [];
		let disembarks: DisembarkAnim[] = [];
		let landerX = 0;
		let landerY = 0;
		let landerVx = 0;
		let landerVy = 0;
		let landerAngle = 0;
		let thrusting = false;
		let landed = false;
		let landedOnStation = false;
		let fuel = 100;
		let exploding = false;
		let explosionTimer = 0;
		let explosionParts: ExplosionParticle[] = [];
		let mouseX = -1;
		let mouseY = -1;
		const sidebarState: SidebarState = { scrollOffset: 0 };
		let logicalW = 0;
		let logicalH = 0;
		const keys = new Set<string>();
		let frameId = 0;
		let lastTs = 0;

		// Round / progression state.
		const savedNames = new Set<string>();
		let roundCompleteTimer = 0;
		let victory = false;
		let victoryTimer = 0;
		let victoryBursts: ExplosionParticle[][] = [];
		let introTimer = INTRO_DURATION;
		// Sidebar interaction state.
		let savedBubbleTimer = 0;
		let savedBubbleName = "";
		let dialogVisible = false;
		let dialogTarget = "";

		// ---- round management ----
		function startNewRound() {
			const unsaved = shuffle(ROSTER.filter((r) => !savedNames.has(r.name)));
			const pick = unsaved.slice(0, CODERNAUTS_PER_ROUND);
			terrain = generateTerrain(logicalW - SIDEBAR_W, logicalH - DASHBOARD_H);
			codernauts = createCodernauts(terrain.pads, pick);
			disembarks = [];
			resetLander();
			roundCompleteTimer = 0;
		}

		function resetLander() {
			landerX = (logicalW - SIDEBAR_W) / 2;
			landerY = logicalH * 0.15;
			landerVx = 0;
			landerVy = 0;
			landerAngle = 0;
			thrusting = false;
			landed = false;
			landedOnStation = false;
			exploding = false;
			explosionTimer = 0;
			explosionParts = [];
			fuel = 100;
			for (const n of codernauts) {
				if (n.aboard) n.aboard = false;
			}
		}

		// ---- sizing ----
		function resize() {
			const dpr = window.devicePixelRatio || 1;
			const rect = ctr.getBoundingClientRect();
			logicalW = rect.width;
			logicalH = rect.height;
			cvs.width = logicalW * dpr;
			cvs.height = logicalH * dpr;
			cvs.style.width = `${logicalW}px`;
			cvs.style.height = `${logicalH}px`;
			cx.setTransform(dpr, 0, 0, dpr, 0, 0);
			if (!victory) startNewRound();
		}

		// ---- input ----
		function onKeyDown(e: KeyboardEvent) {
			if (
				e.key === "ArrowLeft" ||
				e.key === "ArrowRight" ||
				e.key === "ArrowUp" ||
				e.key === "ArrowDown"
			) {
				e.preventDefault();
			}
			keys.add(e.key);
		}
		function onKeyUp(e: KeyboardEvent) {
			keys.delete(e.key);
		}
		function onMouseMove(e: MouseEvent) {
			const rect = cvs.getBoundingClientRect();
			mouseX = e.clientX - rect.left;
			mouseY = e.clientY - rect.top;
		}
		function onMouseLeave() {
			mouseX = -1;
			mouseY = -1;
		}
		function onWheel(e: WheelEvent) {
			const sbx = logicalW - SIDEBAR_W;
			if (mouseX >= sbx) {
				e.preventDefault();
				const totalH = ROSTER.length * 16 * 1.6;
				const viewH = logicalH - DASHBOARD_H - 24;
				const maxScroll = Math.max(0, totalH - viewH);
				sidebarState.scrollOffset = Math.max(
					0,
					Math.min(maxScroll, sidebarState.scrollOffset + e.deltaY),
				);
			}
		}
		function onClick(e: MouseEvent) {
			const rect = cvs.getBoundingClientRect();
			const mx = e.clientX - rect.left;
			const my = e.clientY - rect.top;

			// Handle dialog button clicks first.
			if (dialogVisible) {
				const dlgX = (logicalW - SIDEBAR_W) / 2;
				const dlgY = (logicalH - DASHBOARD_H) / 2;
				const btnW = 60;
				const btnH = 22;
				const yesX = dlgX - btnW - 10;
				const noX = dlgX + 10;
				const btnY = dlgY + 10;
				if (
					mx >= yesX &&
					mx <= yesX + btnW &&
					my >= btnY &&
					my <= btnY + btnH
				) {
					// Yes - travel to another base with this Codernaut.
					const target = dialogTarget;
					dialogVisible = false;
					const entry = ROSTER.find((r) => r.name === target);
					if (entry) {
						const unsaved = shuffle(
							ROSTER.filter(
								(r) => !savedNames.has(r.name) && r.name !== target,
							),
						);
						const pick = [entry, ...unsaved.slice(0, CODERNAUTS_PER_ROUND - 1)];
						terrain = generateTerrain(
							logicalW - SIDEBAR_W,
							logicalH - DASHBOARD_H,
						);
						codernauts = createCodernauts(terrain.pads, pick);
						disembarks = [];
						resetLander();
						roundCompleteTimer = 0;
					}
				} else if (
					mx >= noX &&
					mx <= noX + btnW &&
					my >= btnY &&
					my <= btnY + btnH
				) {
					dialogVisible = false;
				}
				return;
			}

			const name = sidebarHitTest(
				mx,
				my,
				logicalW,
				logicalH,
				sidebarState.scrollOffset,
			);
			if (!name) return;

			// Saved - show bubble over the base.
			if (savedNames.has(name)) {
				savedBubbleTimer = 2.5;
				savedBubbleName = name;
				return;
			}

			// On screen - make them wave.
			const onScreen = codernauts.find(
				(c) => c.name === name && !c.aboard && !c.saved,
			);
			if (onScreen) {
				onScreen.spotlight = 2.5;
				onScreen.spotlightPhase = 0;
				onScreen.waving = true;
				onScreen.waveTimer = 2.5;
				onScreen.wavePhase = 0;
				return;
			}

			// Another base - show confirmation dialog.
			dialogTarget = name;
			dialogVisible = true;
		}

		// ---- game loop ----
		function loop(ts: number) {
			const dt = lastTs ? (ts - lastTs) / 1000 : 0;
			lastTs = ts;

			const dashW = logicalW - SIDEBAR_W;

			// --- intro screen ---
			if (introTimer > 0) {
				introTimer -= dt;
				cx.fillStyle = "black";
				cx.fillRect(0, 0, logicalW, logicalH);

				const playH = logicalH - DASHBOARD_H;
				drawVectorText(cx, "SAVE ALL", dashW / 2, playH * 0.25, 44);
				drawVectorText(cx, "CODERNAUTS", dashW / 2, playH * 0.43, 44);

				cx.fillStyle = "white";
				cx.font = "bold 14px monospace";
				cx.textAlign = "center";
				cx.fillText(
					"press \u2190 to rotate left, press \u2192 to rotate right",
					dashW / 2,
					playH * 0.59,
				);
				cx.fillText("press \u2193 for main thruster", dashW / 2, playH * 0.65);
				if (Math.floor(introTimer * 2.5) % 2 === 0) {
					cx.fillStyle = "#888";
					cx.font = "12px monospace";
					cx.fillText(
						`get ready... ${Math.ceil(introTimer)}`,
						dashW / 2,
						playH * 0.76,
					);
				}

				drawDashboard(
					cx,
					dashW,
					logicalH,
					{ velocity: 0, yaw: 0, altitude: 0, fuel: 100 },
					[],
					0,
				);
				frameId = requestAnimationFrame(loop);
				return;
			}

			// --- victory screen ---
			if (victory) {
				victoryTimer -= dt;
				for (const b of victoryBursts) updateExplosion(b, dt);

				cx.fillStyle = "black";
				cx.fillRect(0, 0, logicalW, logicalH);

				const playH = logicalH - DASHBOARD_H;
				drawTada(cx, dashW / 2, playH * 0.4);

				const prog = Math.max(0, victoryTimer / VICTORY_DURATION);
				for (const b of victoryBursts) drawExplosion(cx, b, prog);

				cx.fillStyle = "#aaa";
				cx.font = "14px monospace";
				cx.textAlign = "center";
				cx.fillText("All Codernauts have been saved!", dashW / 2, playH * 0.6);

				const savedPct = (savedNames.size / ROSTER.length) * 100;
				drawDashboard(
					cx,
					dashW,
					logicalH,
					{ velocity: 0, yaw: 0, altitude: 0, fuel: 100 },
					[],
					savedPct,
				);
				frameId = requestAnimationFrame(loop);
				return;
			}

			// --- round-complete transition ---
			if (roundCompleteTimer > 0) {
				roundCompleteTimer -= dt;
				if (roundCompleteTimer <= 0) {
					if (savedNames.size >= ROSTER.length) {
						victory = true;
						victoryTimer = VICTORY_DURATION;
						victoryBursts = createCelebrationExplosions(
							dashW,
							logicalH - DASHBOARD_H,
						);
					} else {
						startNewRound();
					}
				}
			}

			// -- update codernauts --
			if (terrain) updateCodernauts(codernauts, terrain.pads, dt);

			if (exploding) {
				explosionTimer -= dt;
				updateExplosion(explosionParts, dt);
				if (explosionTimer <= 0) resetLander();
			} else if (roundCompleteTimer <= 0) {
				if (landed) {
					if (landedOnStation && fuel < 100) {
						fuel = Math.min(100, fuel + FUEL_REFILL_RATE * dt);
					}
					thrusting = keys.has("ArrowDown") && fuel > 0;
					if (thrusting) {
						fuel = Math.max(0, fuel - FUEL_BURN_MAIN * dt);
						landerVx += Math.sin(landerAngle) * THRUST_ACCEL * dt;
						landerVy += -Math.cos(landerAngle) * THRUST_ACCEL * dt;
						landerX += landerVx * dt;
						landerY += landerVy * dt;
						landed = false;
						landedOnStation = false;
					}
				} else {
					const rotating =
						(keys.has("ArrowLeft") || keys.has("ArrowRight")) && fuel > 0;
					if (rotating) {
						if (keys.has("ArrowLeft")) landerAngle -= ROTATION_SPEED * dt;
						if (keys.has("ArrowRight")) landerAngle += ROTATION_SPEED * dt;
						fuel = Math.max(0, fuel - FUEL_BURN_ROT * dt);
					}
					thrusting = keys.has("ArrowDown") && fuel > 0;
					if (thrusting) {
						fuel = Math.max(0, fuel - FUEL_BURN_MAIN * dt);
						landerVx += Math.sin(landerAngle) * THRUST_ACCEL * dt;
						landerVy += -Math.cos(landerAngle) * THRUST_ACCEL * dt;
					}
					landerVy += GRAVITY * dt;
					landerX += landerVx * dt;
					landerY += landerVy * dt;

					if (
						terrain &&
						landerHitsTerrain(landerX, landerY, landerAngle, terrain.points)
					) {
						const pad = tryLanding(
							landerX,
							landerY,
							landerAngle,
							landerVx,
							landerVy,
							terrain.pads,
						);
						if (pad) {
							landed = true;
							landerVx = 0;
							landerVy = 0;
							landerAngle = 0;
							thrusting = false;
							landerY = pad.y - 8;
							landedOnStation = pad.isStation;

							if (pad.isStation) {
								const stationCX = pad.x + pad.width / 2;
								let offset = -20;
								for (const n of codernauts) {
									if (!n.aboard) continue;
									n.aboard = false;
									n.saved = true;
									savedNames.add(n.name);
									disembarks.push({
										name: n.name,
										x: landerX + offset,
										targetX: stationCX,
										groundY: pad.y,
										phase: 0,
										waving: false,
										waveTimer: 0,
										wavePhase: 0,
										done: false,
									});
									offset += 12;
								}
							} else {
								const aboardCount = codernauts.filter((c) => c.aboard).length;
								let seats = 4 - aboardCount;
								for (const n of codernauts) {
									if (seats <= 0) break;
									if (n.aboard || n.saved) continue;
									if (n.padIdx !== terrain.pads.indexOf(pad)) continue;
									n.aboard = true;
									seats--;
								}
							}

							if (codernauts.every((c) => c.saved)) {
								roundCompleteTimer = ROUND_TRANSITION_DELAY;
							}
						} else {
							exploding = true;
							explosionTimer = EXPLOSION_DURATION;
							explosionParts = createExplosion(landerX, landerY);
						}
					}
				}
			}

			// -- draw --
			cx.fillStyle = "black";
			cx.fillRect(0, 0, logicalW, logicalH);

			if (terrain) {
				drawTerrain(cx, terrain);
				for (const naut of codernauts) {
					if (!naut.aboard && !naut.saved) {
						drawCodernaut(cx, naut, terrain.pads[naut.padIdx].y);
					}
				}
			}

			updateDisembarks(disembarks, dt);
			disembarks = disembarks.filter((a) => !a.done);
			for (const a of disembarks) {
				const tmpNaut: Codernaut = {
					x: a.x,
					padIdx: 0,
					dir: a.targetX > a.x ? 1 : -1,
					speed: 0,
					walkPhase: a.phase,
					waving: a.waving,
					waveTimer: a.waveTimer,
					wavePhase: a.wavePhase,
					nextWave: 99,
					name: a.name,
					role: "",
					aboard: false,
					saved: false,
					spotlight: 0,
					spotlightPhase: 0,
				};
				drawCodernaut(cx, tmpNaut, a.groundY);
			}

			if (exploding) {
				const alpha = Math.max(0, explosionTimer / EXPLOSION_DURATION);
				drawExplosion(cx, explosionParts, alpha);
			} else {
				drawLander(cx, landerX, landerY, landerAngle, thrusting);
			}

			// Round-complete tooltip at the base.
			if (roundCompleteTimer > 0 && terrain) {
				const station = terrain.pads.find((p) => p.isStation);
				if (station) {
					const scx = station.x + station.width / 2;
					drawTooltip(
						cx,
						"Everyone saved here!",
						"Onto the next base...",
						scx,
						station.y - 40,
					);
				}
			}

			// Hover tooltip.
			if (terrain && mouseX >= 0) {
				for (const naut of codernauts) {
					if (naut.aboard || naut.saved) continue;
					const padY = terrain.pads[naut.padIdx].y;
					const dx = mouseX - naut.x;
					const dy = mouseY - (padY - 5);
					if (dx * dx + dy * dy < 15 * 15) {
						drawTooltip(cx, naut.name, naut.role, naut.x, padY - 14);
						break;
					}
				}
			}

			// Spotlight tooltip ("It's me").
			if (terrain) {
				for (const naut of codernauts) {
					if (naut.spotlight > 0 && !naut.aboard && !naut.saved) {
						const padY = terrain.pads[naut.padIdx].y;
						const jy =
							-Math.abs(Math.sin(naut.spotlightPhase * Math.PI * 3)) * 10;
						drawTooltip(cx, "It's me!", naut.name, naut.x, padY + jy - 14);
					}
				}
			}

			// Sidebar roster.
			const currentNautNames = new Set(
				codernauts.filter((c) => !c.aboard && !c.saved).map((c) => c.name),
			);
			drawSidebar(
				cx,
				logicalW,
				logicalH,
				savedNames,
				currentNautNames,
				sidebarState,
			);

			// Sidebar hover tooltip.
			if (mouseX >= logicalW - SIDEBAR_W) {
				const hName = sidebarHitTest(
					mouseX,
					mouseY,
					logicalW,
					logicalH,
					sidebarState.scrollOffset,
				);
				if (hName) {
					const entry = ROSTER.find((r) => r.name === hName);
					if (entry) {
						let statusLine: string;
						if (savedNames.has(entry.name)) {
							statusLine = "\u2713 saved";
						} else if (currentNautNames.has(entry.name)) {
							statusLine = "\u26a0 requesting help";
						} else {
							statusLine = "\u2026 another base";
						}
						drawTooltip(
							cx,
							`${entry.name} \u2014 ${statusLine}`,
							entry.role,
							mouseX,
							mouseY,
						);
					}
				}
			}

			// Dashboard.
			const speed = Math.sqrt(landerVx * landerVx + landerVy * landerVy);
			const surfaceBelow = terrain
				? terrainHeightAt(landerX, terrain.points)
				: logicalH;
			const altitude = surfaceBelow - (landerY + 8);
			const cargo = codernauts.filter((c) => c.aboard);
			const savedPct = (savedNames.size / ROSTER.length) * 100;

			// "Already saved" bubble over the base.
			if (savedBubbleTimer > 0) {
				savedBubbleTimer -= dt;
				if (terrain) {
					const station = terrain.pads.find((p) => p.isStation);
					if (station) {
						const scx = station.x + station.width / 2;
						drawTooltip(
							cx,
							"I am already saved!",
							savedBubbleName,
							scx,
							station.y - 40,
						);
					}
				}
			}

			// Confirmation dialog for travelling to another base.
			if (dialogVisible) {
				const dlgX = dashW / 2;
				const dlgY = (logicalH - DASHBOARD_H) / 2;
				const dlgW = 340;
				const dlgH = 70;
				const r = 8;

				// Backdrop dim.
				cx.fillStyle = "rgba(0,0,0,0.5)";
				cx.fillRect(0, 0, dashW, logicalH - DASHBOARD_H);

				// Dialog box.
				cx.fillStyle = "#181818";
				cx.strokeStyle = "#888";
				cx.lineWidth = 1.5;
				cx.beginPath();
				cx.roundRect(dlgX - dlgW / 2, dlgY - dlgH / 2, dlgW, dlgH, r);
				cx.fill();
				cx.stroke();

				cx.fillStyle = "white";
				cx.font = "bold 11px monospace";
				cx.textAlign = "center";
				cx.fillText(`Travel to save ${dialogTarget}?`, dlgX, dlgY - 8);

				// Buttons.
				const btnW = 60;
				const btnH = 22;
				const btnY = dlgY + 10;
				for (const [label, bx] of [
					["YES", dlgX - btnW - 10],
					["NO", dlgX + 10],
				] as const) {
					cx.strokeStyle = "#666";
					cx.lineWidth = 1;
					cx.beginPath();
					cx.roundRect(bx, btnY, btnW, btnH, 4);
					cx.stroke();
					cx.fillStyle = "white";
					cx.font = "bold 10px monospace";
					cx.fillText(label, bx + btnW / 2, btnY + 15);
				}
			}

			drawDashboard(
				cx,
				dashW,
				logicalH,
				{
					velocity: speed,
					yaw: normalizeAngle(landerAngle),
					altitude,
					fuel,
				},
				cargo,
				savedPct,
			);

			frameId = requestAnimationFrame(loop);
		}

		// ---- bootstrap ----
		const observer = new ResizeObserver(resize);
		observer.observe(ctr);
		window.addEventListener("keydown", onKeyDown);
		window.addEventListener("keyup", onKeyUp);
		cvs.addEventListener("mousemove", onMouseMove);
		cvs.addEventListener("mouseleave", onMouseLeave);
		cvs.addEventListener("wheel", onWheel, { passive: false });
		cvs.addEventListener("click", onClick);

		resize();
		frameId = requestAnimationFrame(loop);

		return () => {
			cancelAnimationFrame(frameId);
			observer.disconnect();
			window.removeEventListener("keydown", onKeyDown);
			window.removeEventListener("keyup", onKeyUp);
			cvs.removeEventListener("mousemove", onMouseMove);
			cvs.removeEventListener("mouseleave", onMouseLeave);
			cvs.removeEventListener("wheel", onWheel);
			cvs.removeEventListener("click", onClick);
		};
	}, []);

	return (
		<div ref={containerRef} className="w-full h-full bg-black">
			<canvas ref={canvasRef} className="block" />
		</div>
	);
};
