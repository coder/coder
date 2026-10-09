/**
 * Copied from shadc/ui on 01/13/2025
 * @see {@link https://ui.shadcn.com/docs/components/chart}
 */

import { cn } from "cn";
import {
	createContext,
	useContext,
	useEffect,
	useId,
	useMemo,
	useRef,
	useState,
} from "react";
import * as RechartsPrimitive from "recharts";
import { formatDate } from "#/utils/time";

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
	setAnnouncement: (announcement: string) => void;
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
	const [announcement, setAnnouncement] = useState("");

	return (
		<ChartContext.Provider value={{ config, setAnnouncement }}>
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
				{/*
					The tooltip is hidden while inactive, so it can't be a live region.
					This element stays mounted and mirrors the active tooltip's text so
					screen readers announce each point as users move through the chart.
				*/}
				<div
					role="status"
					className="sr-only"
					aria-live="polite"
					aria-atomic="true"
				>
					{announcement}
				</div>
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

/**
 * Builds the `desc` for a daily chart: a summary of the date range and a hint
 * about the keyboard navigation provided by recharts' `accessibilityLayer`.
 */
export function getDailyChartDescription(
	summary: string,
	dates: readonly string[],
): string {
	const hint = "Use the left and right arrow keys to read each day.";
	if (dates.length === 0) {
		return `${summary}. ${hint}`;
	}
	const format = (date: string) =>
		formatDate(new Date(date), {
			hour: undefined,
			minute: undefined,
			second: undefined,
		});
	return `${summary} from ${format(dates[0])} to ${format(dates[dates.length - 1])}. ${hint}`;
}

export const ChartTooltip = RechartsPrimitive.Tooltip;

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
}) => {
	const { config, setAnnouncement } = useChart();
	const contentRef = useRef<HTMLDivElement>(null);
	const isActive = Boolean(active && payload?.length);

	// Mirror the rendered tooltip text into the container's live region. React
	// skips the update when the text is unchanged, so moving within one data
	// point doesn't repeat the announcement.
	useEffect(() => {
		setAnnouncement(
			isActive && contentRef.current ? getSpokenText(contentRef.current) : "",
		);
	});

	// Clear the announcement when the tooltip is removed from the chart.
	useEffect(() => () => setAnnouncement(""), [setAnnouncement]);

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

	if (!isActive || !payload) {
		return null;
	}

	const nestLabel = payload.length === 1 && indicator !== "dot";

	return (
		<div
			ref={contentRef}
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

// Joins an element's text nodes with spaces. `textContent` would run adjacent
// blocks together ("0 usersJuly 26").
function getSpokenText(element: HTMLElement): string {
	const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
	const parts: string[] = [];
	for (let node = walker.nextNode(); node; node = walker.nextNode()) {
		const text = node.textContent?.trim();
		if (text) {
			parts.push(text);
		}
	}
	return parts.join(" ");
}

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
