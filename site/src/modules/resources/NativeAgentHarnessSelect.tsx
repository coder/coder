import { useId, useRef } from "react";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import type { NativeAgentHarness } from "./nativeAgentMetadata";

type NativeAgentHarnessSelectProps = {
	harnesses: readonly NativeAgentHarness[];
	value: string | undefined;
	unavailable: boolean;
	onValueChange: (harness: string) => void;
	onCloseWithoutTrigger: () => void;
};

/** Keeps an unavailable explicit choice visible until the user selects a replacement. */
export const NativeAgentHarnessSelect: React.FC<
	NativeAgentHarnessSelectProps
> = ({
	harnesses,
	value,
	unavailable,
	onValueChange,
	onCloseWithoutTrigger,
}) => {
	const id = useId();
	const triggerRef = useRef<HTMLButtonElement>(null);
	if (harnesses.length < 2 && !unavailable) return null;

	return (
		<div className="w-full space-y-2 sm:w-auto">
			<Label htmlFor={id}>Harness</Label>
			<Select value={value ?? ""} onValueChange={onValueChange}>
				<SelectTrigger id={id} ref={triggerRef}>
					<SelectValue placeholder="Select a harness" />
				</SelectTrigger>
				<SelectContent
					onCloseAutoFocus={(event) => {
						if (!triggerRef.current?.isConnected) {
							event.preventDefault();
							onCloseWithoutTrigger();
						}
					}}
				>
					{unavailable && value && (
						<SelectItem value={value} disabled>
							{value} (unavailable)
						</SelectItem>
					)}
					{harnesses.map((item) => (
						<SelectItem key={item.id} value={item.id}>
							{item.title}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	);
};
