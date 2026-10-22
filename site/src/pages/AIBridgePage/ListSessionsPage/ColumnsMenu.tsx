import { RestrictToVerticalAxis } from "@dnd-kit/abstract/modifiers";
import {
	type DragDropManager,
	DragDropProvider,
	type DragEndEvent,
} from "@dnd-kit/react";
import { isSortable, useSortable } from "@dnd-kit/react/sortable";
import { cn } from "cn";
import {
	CheckIcon,
	CoinsIcon,
	Columns3CogIcon,
	Columns3Icon,
	GripVerticalIcon,
	type LucideIcon,
	ShieldCheckIcon,
} from "lucide-react";
import { Fragment, useId, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	getActiveView,
	moveShownColumn,
	type SessionColumnId,
	type SessionColumnSettings,
	type SessionColumnViewId,
	sessionColumns,
	sessionColumnViews,
	toggleColumn,
} from "./sessionColumns";

const viewIcons: Record<SessionColumnViewId, LucideIcon> = {
	general: Columns3Icon,
	security: ShieldCheckIcon,
	cost: CoinsIcon,
};

// dnd-kit moves the dragged row's element while sorting, which takes focus
// from a keyboard drag, so give it back to the handle once that renders.
const refocusKeyboardDrag = (manager: DragDropManager) => {
	const { source, activatorEvent } = manager.dragOperation;
	const handle = source?.handle;
	if (
		activatorEvent instanceof KeyboardEvent &&
		isSortable(source) &&
		handle instanceof HTMLElement
	) {
		void manager.renderer.rendering.then(() =>
			setTimeout(() => handle.focus()),
		);
	}
};

type ColumnsMenuProps = {
	settings: SessionColumnSettings;
	onSettingsChange: (settings: SessionColumnSettings) => void;
	fittingCount: number;
};

/** Controlled menu: settings come from props and every change goes through `onSettingsChange`. */
export const ColumnsMenu: React.FC<ColumnsMenuProps> = ({
	settings,
	onSettingsChange,
	fittingCount,
}) => {
	const [isDragging, setIsDragging] = useState(false);

	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button
					variant="outline"
					size="sm"
					className="shrink-0 text-content-secondary"
				>
					<Columns3CogIcon />
					Customize view
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="flex flex-col"
				// Escape cancels a keyboard drag; keep the menu open for it.
				onEscapeKeyDown={(event) => {
					if (isDragging) {
						event.preventDefault();
					}
				}}
			>
				<ViewList settings={settings} onSettingsChange={onSettingsChange} />
				<div className="max-h-96 overflow-y-auto p-2">
					<ShownColumnList
						settings={settings}
						onSettingsChange={onSettingsChange}
						fittingCount={fittingCount}
						onDraggingChange={setIsDragging}
					/>
					<NotShownColumnList
						settings={settings}
						onSettingsChange={onSettingsChange}
					/>
				</div>
			</PopoverContent>
		</Popover>
	);
};

type ColumnListProps = {
	settings: SessionColumnSettings;
	onSettingsChange: (settings: SessionColumnSettings) => void;
};

const ViewList: React.FC<ColumnListProps> = ({
	settings,
	onSettingsChange,
}) => {
	const headingId = useId();
	const activeView = getActiveView(settings);

	return (
		<section
			aria-labelledby={headingId}
			className="border-0 border-b border-solid border-border p-2"
		>
			<h3
				id={headingId}
				className="m-0 px-2 py-1.5 text-sm font-normal text-content-secondary"
			>
				Views
			</h3>
			{sessionColumnViews.map((view) => {
				const Icon = viewIcons[view.id];
				const isActive = activeView?.id === view.id;
				return (
					<button
						key={view.id}
						type="button"
						aria-pressed={isActive}
						onClick={() => onSettingsChange(view.settings)}
						className={cn(
							"flex w-full cursor-pointer items-center gap-3 rounded-md border-0 bg-transparent px-2 py-2 text-left text-sm",
							"hover:bg-surface-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
							isActive ? "text-content-primary" : "text-content-secondary",
						)}
					>
						<Icon className="size-icon-sm shrink-0" />
						<span className="flex-1">{view.label}</span>
						{isActive && <CheckIcon className="size-icon-sm shrink-0" />}
					</button>
				);
			})}
		</section>
	);
};

type ShownColumnListProps = ColumnListProps & {
	fittingCount: number;
	onDraggingChange: (isDragging: boolean) => void;
};

const ShownColumnList: React.FC<ShownColumnListProps> = ({
	settings,
	onSettingsChange,
	fittingCount,
	onDraggingChange,
}) => {
	const handleDragEnd = ({ operation, canceled }: DragEndEvent) => {
		onDraggingChange(false);
		const { source } = operation;
		if (canceled || !isSortable(source)) {
			return;
		}
		// dnd-kit has already moved the row in the DOM; source.index is where
		// it was dropped.
		const id = settings.shown.find((c) => c === source.id);
		if (id) {
			onSettingsChange(moveShownColumn(settings, id, source.index));
		}
	};

	const onlyShownColumn =
		settings.shown.length === 1 ? settings.shown[0] : undefined;

	return (
		<DragDropProvider
			modifiers={[RestrictToVerticalAxis]}
			onDragStart={() => onDraggingChange(true)}
			onDragOver={(_event, manager) => refocusKeyboardDrag(manager)}
			onDragEnd={handleDragEnd}
		>
			<ul className="m-0 list-none p-0">
				{settings.shown.map((id, i) => (
					<Fragment key={id}>
						{/* A sibling of the rows, not part of one, so it stays put while
						    a row is dragged past it. */}
						{i === fittingCount && <HiddenAtWidthDivider />}
						<SortableColumnRow
							id={id}
							index={i}
							isLocked={id === onlyShownColumn}
							onToggle={() => onSettingsChange(toggleColumn(settings, id))}
						/>
					</Fragment>
				))}
			</ul>
		</DragDropProvider>
	);
};

const NotShownColumnList: React.FC<ColumnListProps> = ({
	settings,
	onSettingsChange,
}) => {
	const headingId = useId();
	if (settings.notShown.length === 0) {
		return null;
	}
	return (
		<section aria-labelledby={headingId}>
			<h3
				id={headingId}
				className="m-0 px-2 pb-1.5 pt-3 text-sm font-normal text-content-secondary"
			>
				Not shown
			</h3>
			<ul className="m-0 list-none p-0">
				{settings.notShown.map((id) => (
					<li key={id}>
						<ColumnToggle
							id={id}
							checked={false}
							onToggle={() => onSettingsChange(toggleColumn(settings, id))}
							className="pl-8 text-content-secondary"
						/>
					</li>
				))}
			</ul>
		</section>
	);
};

const HiddenAtWidthDivider: React.FC = () => (
	<li className="flex items-center gap-2 px-2 py-1.5 text-xs text-content-secondary">
		<span className="h-px flex-1 bg-border" />
		Hidden at this width
		<span className="h-px flex-1 bg-border" />
	</li>
);

type SortableColumnRowProps = {
	id: SessionColumnId;
	index: number;
	/** The last shown column cannot be hidden. */
	isLocked: boolean;
	onToggle: () => void;
};

const SortableColumnRow: React.FC<SortableColumnRowProps> = ({
	id,
	index,
	isLocked,
	onToggle,
}) => {
	const { ref, handleRef, isDragging } = useSortable({ id, index });
	const { label } = sessionColumns[id];

	return (
		<li
			ref={ref}
			className={cn("relative bg-surface-primary", isDragging && "z-10")}
		>
			<div className="flex items-center">
				<button
					ref={handleRef}
					type="button"
					aria-label={`Reorder ${label}`}
					className="flex size-6 shrink-0 cursor-grab touch-none items-center justify-center rounded-sm border-0 bg-transparent p-0 text-content-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link active:cursor-grabbing"
				>
					<GripVerticalIcon className="size-icon-sm" />
				</button>
				<ColumnToggle
					id={id}
					checked
					isLocked={isLocked}
					onToggle={onToggle}
					className="pl-2"
				/>
			</div>
		</li>
	);
};

type ColumnToggleProps = {
	id: SessionColumnId;
	checked: boolean;
	isLocked?: boolean;
	onToggle: () => void;
	className: string;
};

const ColumnToggle: React.FC<ColumnToggleProps> = ({
	id,
	checked,
	isLocked,
	onToggle,
	className,
}) => {
	const switchId = useId();
	// aria-disabled instead of disabled keeps the switch focusable so keyboard
	// users can reach the tooltip explaining why it cannot be turned off.
	const toggle = (
		<Switch
			id={switchId}
			checked={checked}
			aria-disabled={isLocked}
			onCheckedChange={() => {
				if (!isLocked) {
					onToggle();
				}
			}}
			className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
		/>
	);
	return (
		<div
			className={cn(
				"flex min-w-0 flex-1 items-center gap-2 py-1.5 pr-2 text-sm",
				className,
			)}
		>
			<label htmlFor={switchId} className="min-w-0 flex-1 truncate">
				{sessionColumns[id].label}
			</label>
			{isLocked ? (
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>{toggle}</TooltipTrigger>
						<TooltipContent side="left">
							At least one column must be shown.
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			) : (
				toggle
			)}
		</div>
	);
};
