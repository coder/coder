/**
 * Copied from shadc/ui on 01/13/2025
 * @see {@link https://ui.shadcn.com/docs/components/chart}
 */

import { cn } from "cn";
import {
	cloneElement,
	createContext,
	useCallback,
	useContext,
	useId,
	useMemo,
	useState,
} from "react";
import * as RechartsPrimitive from "recharts";

// Format: { THEME_NAME: CSS_SELECTOR }
const THEMES = { light: "", dark: ".dark" } as const;

export type ChartConfig = {
	[k in string]: {
		label?: React.ReactNode;
		icon?: React.ComponentType;
	} & (
		| { color?: string; theme?: never }
		| { color?: never; theme: Record<keyof typeof THEMES, string> }
	);
};

type ChartContextProps = {
	config: ChartConfig;
};

const ChartContext = createContext<ChartContextProps | null>(null);

function useChart() {
	const context = useContext(ChartContext);

	if (!context) {
		throw new Error("useChart must be used within a <ChartContainer />");
	}

	return context;
}

type ChartContainerProps = Omit<React.ComponentProps<"div">, "children"> &
	Pick<
		React.ComponentProps<typeof RechartsPrimitive.ResponsiveContainer>,
		"children"
	> & {
		config: ChartConfig;
	};

export const ChartContainer: React.FC<ChartContainerProps> = ({
	id,
	className,
	children,
	config,
	...props
}) => {
	const uniqueId = useId();
	const chartId = `chart-${id || uniqueId.replace(/:/g, "")}`;

	return (
		<ChartContext.Provider value={{ config }}>
			<div
				data-chart={chartId}
				className={cn(
					"flex aspect-video justify-center text-xs",
					"[&_.recharts-cartesian-axis-tick_text]:fill-muted-foreground",
					"[&_.recharts-cartesian-grid_line[stroke='#ccc']]:stroke-border/50",
					"[&_.recharts-curve.recharts-tooltip-cursor]:stroke-border",
					"[&_.recharts-dot[stroke='#fff']]:stroke-transparent",
					"[&_.recharts-layer]:outline-hidden",
					"[&_.recharts-polar-grid_[stroke='#ccc']]:stroke-border",
					"[&_.recharts-radial-bar-background-sector]:fill-muted",
					"[&_.recharts-rectangle.recharts-tooltip-cursor]:fill-muted",
					"[&_.recharts-reference-line_[stroke='#ccc']]:stroke-border",
					"[&_.recharts-sector[stroke='#fff']]:stroke-transparent",
					"[&_.recharts-sector]:outline-hidden",
					"[&_.recharts-surface]:outline-hidden",
					"[&_.recharts-text]:fill-content-secondary [&_.recharts-text]:font-medium",
					"[&_.recharts-cartesian-axis-line]:stroke-[hsl(var(--border-default))]",
					className,
				)}
				{...props}
			>
				<ChartStyle id={chartId} config={config} />
				<RechartsPrimitive.ResponsiveContainer>
					{children}
				</RechartsPrimitive.ResponsiveContainer>
			</div>
		</ChartContext.Provider>
	);
};

const ChartStyle = ({ id, config }: { id: string; config: ChartConfig }) => {
	const colorConfig = Object.entries(config).filter(
		([, config]) => config.theme || config.color,
	);

	if (!colorConfig.length) {
		return null;
	}

	return (
		<style
			// oxlint-disable-next-line react/no-danger -- This renders generated chart CSS, which is safe in this case because we control the inputs
			dangerouslySetInnerHTML={{
				__html: Object.entries(THEMES)
					.map(
						([theme, prefix]) => `
${prefix} [data-chart=${id}] {
${colorConfig
	.map(([key, itemConfig]) => {
		const color =
			itemConfig.theme?.[theme as keyof typeof itemConfig.theme] ||
			itemConfig.color;
		return color ? `  --color-${key}: ${color};` : null;
	})
	.join("\n")}
}`,
					)
					.join("\n"),
			}}
		/>
	);
};

type ChartTooltipProps = Omit<
	React.ComponentProps<typeof RechartsPrimitive.Tooltip>,
	"content"
> & {
	content: React.ReactElement;
};

type FrozenTooltipState = Pick<
	ChartTooltipProps,
	"label" | "payload" | "coordinate"
>;

/**
 * Recharts tooltip that satisfies WCAG 1.4.13 (content on hover or focus).
 * It is centered above the active data point at the top of the plot, so the
 * pointer can move straight up onto it, and it keeps showing the same data
 * while the pointer is over it. Recharts dismisses it on Escape.
 */
export const ChartTooltip: React.FC<ChartTooltipProps> = ({
	label,
	payload,
	coordinate,
	viewBox,
	content,
	...props
}) => {
	const [frozen, setFrozen] = useState<FrozenTooltipState | null>(null);
	const [width, setWidth] = useState(0);
	// A stable callback ref keeps React from recreating the observer on every
	// render.
	const measureContent = useCallback((node: HTMLDivElement | null) => {
		if (!node) {
			return;
		}
		const observer = new ResizeObserver(() => {
			// Keep the last width while the content is hidden and empty.
			if (node.offsetWidth > 0) {
				setWidth(node.offsetWidth);
			}
		});
		observer.observe(node);
		return () => observer.disconnect();
	}, []);

	const current = frozen ?? { label, payload, coordinate };
	const plotX = viewBox?.x ?? 0;
	const plotWidth = viewBox?.width ?? 0;
	const centeredX = (current.coordinate?.x ?? 0) - width / 2;
	const x = Math.max(
		plotX,
		Math.min(centeredX, plotX + Math.max(plotWidth - width, 0)),
	);

	return (
		<RechartsPrimitive.Tooltip
			isAnimationActive={false}
			{...props}
			{...current}
			active={frozen ? true : props.active}
			viewBox={viewBox}
			position={{ x, y: viewBox?.y ?? 0 }}
			content={
				<HoverableTooltipContent
					containerRef={measureContent}
					innerContent={content}
					onPointerEnter={() => setFrozen({ label, payload, coordinate })}
					onPointerLeave={() => setFrozen(null)}
				/>
			}
		/>
	);
};
// Recharts locates the tooltip among chart children by display name.
ChartTooltip.displayName = "Tooltip";

type HoverableTooltipContentProps = Omit<ChartTooltipProps, "content"> & {
	containerRef: React.Ref<HTMLDivElement>;
	innerContent: React.ReactElement;
	onPointerEnter: () => void;
	onPointerLeave: () => void;
};

// Recharts clones this element with the tooltip props, so it must keep a
// stable component type to avoid remounting under the pointer. Recharts keeps
// the wrapper at pointer-events: none, so only the visible content is
// hoverable and the empty wrapper never blocks the chart.
const HoverableTooltipContent: React.FC<HoverableTooltipContentProps> = ({
	containerRef,
	innerContent,
	onPointerEnter,
	onPointerLeave,
	...props
}) => {
	return (
		<div
			ref={containerRef}
			className="pointer-events-auto"
			onPointerEnter={onPointerEnter}
			onPointerLeave={onPointerLeave}
		>
			{cloneElement(innerContent, props)}
		</div>
	);
};

type ChartTooltipContentProps = React.ComponentProps<
	typeof RechartsPrimitive.Tooltip
> & {
	className?: string;
	color?: string;
	hideLabel?: boolean;
	hideIndicator?: boolean;
	indicator?: "line" | "dot" | "dashed";
	nameKey?: string;
	labelKey?: string;
	ref?: React.Ref<HTMLDivElement>;
};

export const ChartTooltipContent: React.FC<ChartTooltipContentProps> = ({
	active,
	payload,
	formatter,
	className,
	color,
	hideLabel = false,
	hideIndicator = false,
	indicator = "dot",
	nameKey,
	labelKey,
	label,
	labelFormatter,
	labelClassName,
	ref,
}) => {
	const { config } = useChart();

	const tooltipLabel = useMemo(() => {
		if (hideLabel || !payload?.length) {
			return null;
		}

		const [item] = payload;
		const key = `${labelKey || item.dataKey || item.name || "value"}`;
		const itemConfig = getPayloadConfigFromPayload(config, item, key);
		const value =
			!labelKey && typeof label === "string"
				? config[label as keyof typeof config]?.label || label
				: itemConfig?.label;

		if (labelFormatter) {
			return (
				<div className={cn("font-medium", labelClassName)}>
					{labelFormatter(value, payload)}
				</div>
			);
		}

		if (!value) {
			return null;
		}

		return <div className={cn("font-medium", labelClassName)}>{value}</div>;
	}, [
		label,
		labelFormatter,
		payload,
		hideLabel,
		labelClassName,
		config,
		labelKey,
	]);

	if (!active || !payload?.length) {
		return null;
	}

	const nestLabel = payload.length === 1 && indicator !== "dot";

	return (
		<div
			ref={ref}
			className={cn(
				"grid min-w-32 items-start gap-1 rounded-lg border border-solid border-border bg-surface-primary px-3 py-2 text-xs shadow-xl",
				className,
			)}
		>
			{!nestLabel ? tooltipLabel : null}
			<div className="grid gap-1.5">
				{payload.map((item, index) => {
					const key = `${nameKey || item.name || item.dataKey || "value"}`;
					const itemConfig = getPayloadConfigFromPayload(config, item, key);
					const indicatorColor = color || item.payload.fill || item.color;

					return (
						<div
							key={item.dataKey}
							className={cn(
								"flex w-full flex-wrap items-stretch gap-2 [&>svg]:h-2.5 [&>svg]:w-2.5 [&>svg]:text-content-secondary",
								indicator === "dot" && "items-center",
							)}
						>
							{formatter && item?.value !== undefined && item.name ? (
								formatter(item.value, item.name, item, index, item.payload)
							) : (
								<>
									{itemConfig?.icon ? (
										<itemConfig.icon />
									) : (
										!hideIndicator && (
											<div
												className={cn(
													"shrink-0 rounded-[2px] border-(--color-border) bg-(--color-bg)",
													{
														"h-2.5 w-2.5": indicator === "dot",
														"w-1": indicator === "line",
														"w-0 border-[1.5px] border-dashed bg-transparent":
															indicator === "dashed",
														"my-0.5": nestLabel && indicator === "dashed",
													},
												)}
												style={{
													"--color-bg": indicatorColor,
													"--color-border": indicatorColor,
												}}
											/>
										)
									)}
									<div
										className={cn(
											"flex flex-1 justify-between leading-none",
											nestLabel ? "items-end" : "items-center",
										)}
									>
										<div className="grid gap-1.5">
											{nestLabel ? tooltipLabel : null}
											<span className="text-content-secondary">
												{itemConfig?.label || item.name}
											</span>
										</div>
										{item.value && (
											<span className="font-mono font-medium tabular-nums text-content-primary">
												{item.value.toLocaleString()}
											</span>
										)}
									</div>
								</>
							)}
						</div>
					);
				})}
			</div>
		</div>
	);
};

// Helper to extract item config from a payload.
function getPayloadConfigFromPayload(
	config: ChartConfig,
	payload: unknown,
	key: string,
) {
	if (typeof payload !== "object" || payload === null) {
		return undefined;
	}

	const payloadPayload =
		"payload" in payload &&
		typeof payload.payload === "object" &&
		payload.payload !== null
			? payload.payload
			: undefined;

	let configLabelKey: string = key;

	if (
		key in payload &&
		typeof payload[key as keyof typeof payload] === "string"
	) {
		configLabelKey = payload[key as keyof typeof payload] as string;
	} else if (
		payloadPayload &&
		key in payloadPayload &&
		typeof payloadPayload[key as keyof typeof payloadPayload] === "string"
	) {
		configLabelKey = payloadPayload[
			key as keyof typeof payloadPayload
		] as string;
	}

	return configLabelKey in config
		? config[configLabelKey]
		: config[key as keyof typeof config];
}
