import { ArrowLeftIcon, ArrowRightIcon, RotateCwIcon } from "lucide-react";
import { useState } from "react";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Spinner } from "#/components/Spinner/Spinner";
import type { BrowserNavState } from "./protocol";
import type { BrowserStreamStatus } from "./useBrowserStream";

type BrowserToolbarProps = {
	navState: BrowserNavState | undefined;
	status: BrowserStreamStatus;
	addressRef?: React.Ref<HTMLInputElement>;
	onNavigate: (url: string) => void;
	onBack: () => void;
	onForward: () => void;
	onReload: () => void;
};

export const BrowserToolbar: React.FC<BrowserToolbarProps> = ({
	navState,
	status,
	addressRef,
	onNavigate,
	onBack,
	onForward,
	onReload,
}) => {
	// Holds the address bar text while the user edits it; otherwise the bar
	// follows the page URL.
	const [draft, setDraft] = useState<string>();
	const connected = status === "connected";
	const url = navState?.url === "about:blank" ? "" : (navState?.url ?? "");

	return (
		<div className="flex items-center gap-1 border-0 border-b border-solid border-border bg-surface-primary px-2 py-1.5">
			<Button
				variant="subtle"
				size="icon"
				aria-label="Back"
				disabled={!connected || !navState?.canGoBack}
				onClick={onBack}
			>
				<ArrowLeftIcon />
			</Button>
			<Button
				variant="subtle"
				size="icon"
				aria-label="Forward"
				disabled={!connected || !navState?.canGoForward}
				onClick={onForward}
			>
				<ArrowRightIcon />
			</Button>
			<Button
				variant="subtle"
				size="icon"
				aria-label="Reload"
				disabled={!connected}
				onClick={onReload}
			>
				<RotateCwIcon />
			</Button>
			<form
				className="flex flex-1 items-center gap-2"
				onSubmit={(event) => {
					event.preventDefault();
					const value = (draft ?? url).trim();
					if (value) {
						onNavigate(value);
					}
					setDraft(undefined);
				}}
			>
				<Input
					ref={addressRef}
					aria-label="Address"
					className="h-8"
					placeholder="Enter a URL, such as localhost:3000"
					spellCheck={false}
					autoComplete="off"
					disabled={!connected}
					value={draft ?? url}
					onFocus={(event) => {
						setDraft(url);
						event.currentTarget.select();
					}}
					onBlur={() => setDraft(undefined)}
					onChange={(event) => setDraft(event.target.value)}
					onKeyDown={(event) => {
						if (event.key === "Escape") {
							setDraft(undefined);
							event.currentTarget.blur();
						}
					}}
				/>
				{navState?.loading && <Spinner size="sm" loading />}
			</form>
		</div>
	);
};
