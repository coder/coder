import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { ScrollArea } from "./ScrollArea";

const meta: Meta<typeof ScrollArea> = {
	title: "components/ScrollArea",
	component: ScrollArea,
};
export default meta;
type Story = StoryObj<typeof ScrollArea>;

const OverflowingContent = () => (
	<div className="w-[1200px] p-3 font-mono text-xs leading-5">
		{Array.from({ length: 60 }, (_, row) => (
			<div key={row} className="whitespace-nowrap">
				{`row ${row.toString().padStart(2, "0")} `}
				{"value ".repeat(30)}
			</div>
		))}
	</div>
);

const luminance = (color: string): number => {
	const parts = (color.match(/[\d.]+/g) ?? []).map(Number);
	const [r, g, b] = parts.slice(0, 3).map((value) => {
		const channel = value / 255;
		return channel <= 0.03928
			? channel / 12.92
			: ((channel + 0.055) / 1.055) ** 2.4;
	});
	return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};

const contrastRatio = (a: string, b: string): number => {
	const la = luminance(a);
	const lb = luminance(b);
	return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
};

export const Accessibility: Story = {
	render: () => (
		<div
			data-testid="surface"
			className="w-96 rounded-md border border-solid border-border-default bg-surface-primary"
		>
			<ScrollArea
				className="h-48"
				type="always"
				orientation="both"
				scrollBarClassName="w-1.5"
				horizontalScrollBarClassName="h-1.5"
			>
				<OverflowingContent />
			</ScrollArea>
		</div>
	),
	play: async ({ canvasElement }) => {
		const surface = canvasElement.querySelector<HTMLElement>(
			"[data-testid='surface']",
		);
		await expect(surface).not.toBeNull();

		const getThumbs = () => {
			const vertical = canvasElement.querySelector(
				'[data-orientation="vertical"]',
			)?.firstElementChild as HTMLElement | null | undefined;
			const horizontal = canvasElement.querySelector(
				'[data-orientation="horizontal"]',
			)?.firstElementChild as HTMLElement | null | undefined;
			return { vertical, horizontal };
		};

		await waitFor(() => {
			const { vertical, horizontal } = getThumbs();
			expect(vertical).toBeTruthy();
			expect(horizontal).toBeTruthy();
		});

		const { vertical, horizontal } = getThumbs();
		if (!vertical || !horizontal || !surface) {
			throw new Error("scrollbar thumbs not found");
		}

		const verticalBefore = getComputedStyle(vertical, "::before");
		await expect(
			Number.parseFloat(verticalBefore.width),
		).toBeGreaterThanOrEqual(24);
		await expect(
			Number.parseFloat(verticalBefore.height),
		).toBeGreaterThanOrEqual(24);

		const horizontalBefore = getComputedStyle(horizontal, "::before");
		await expect(
			Number.parseFloat(horizontalBefore.width),
		).toBeGreaterThanOrEqual(24);
		await expect(
			Number.parseFloat(horizontalBefore.height),
		).toBeGreaterThanOrEqual(24);

		const thumbColor = getComputedStyle(vertical).backgroundColor;
		const surfaceColor = getComputedStyle(surface).backgroundColor;
		await expect(
			contrastRatio(thumbColor, surfaceColor),
		).toBeGreaterThanOrEqual(3);
	},
};

// type="always" forces the thumb to render so its `::before` hit-target
// pseudo-element is measurable.
export const ThumbHitAreaOverride: Story = {
	render: () => (
		<div className="flex gap-4">
			<div
				data-testid="default-area"
				className="w-48 rounded-md border border-solid border-border-default bg-surface-primary"
			>
				<ScrollArea className="h-48" type="always" scrollBarClassName="w-1.5">
					<OverflowingContent />
				</ScrollArea>
			</div>
			<div
				data-testid="override-area"
				className="w-48 rounded-md border border-solid border-border-default bg-surface-primary"
			>
				<ScrollArea
					className="h-48"
					type="always"
					orientation="both"
					scrollBarClassName="w-1.5"
					horizontalScrollBarClassName="h-1.5"
					scrollThumbClassName="before:hidden"
				>
					<OverflowingContent />
				</ScrollArea>
			</div>
		</div>
	),
	play: async ({ canvasElement }) => {
		const getThumb = (
			testid: string,
			orientation: "vertical" | "horizontal",
		) => {
			const area = canvasElement.querySelector(`[data-testid='${testid}']`);
			return area?.querySelector(`[data-orientation="${orientation}"]`)
				?.firstElementChild as HTMLElement | null | undefined;
		};

		await waitFor(() => {
			expect(getThumb("default-area", "vertical")).toBeTruthy();
			expect(getThumb("override-area", "vertical")).toBeTruthy();
			expect(getThumb("override-area", "horizontal")).toBeTruthy();
		});

		const defaultThumb = getThumb("default-area", "vertical");
		const overrideThumb = getThumb("override-area", "vertical");
		const overrideHorizontalThumb = getThumb("override-area", "horizontal");
		if (!defaultThumb || !overrideThumb || !overrideHorizontalThumb) {
			throw new Error("scrollbar thumbs not found");
		}

		const defaultBefore = getComputedStyle(defaultThumb, "::before");
		await expect(defaultBefore.display).not.toBe("none");
		await expect(Number.parseFloat(defaultBefore.width)).toBeGreaterThanOrEqual(
			24,
		);

		await expect(getComputedStyle(overrideThumb, "::before").display).toBe(
			"none",
		);
		await expect(
			getComputedStyle(overrideHorizontalThumb, "::before").display,
		).toBe("none");
		await expect(overrideThumb.getBoundingClientRect().width).toBeGreaterThan(
			0,
		);
	},
};
