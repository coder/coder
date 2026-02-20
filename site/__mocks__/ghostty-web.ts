// Jest mock for ghostty-web. The real library requires WASM which is
// unavailable in JSDOM, so we provide a minimal DOM-based implementation
// that the test suite can inspect.

type Listener<T> = (arg: T) => void;
type Disposable = { dispose(): void };

function createEvent<T>() {
	const listeners: Listener<T>[] = [];
	const event = (listener: Listener<T>): Disposable => {
		listeners.push(listener);
		return {
			dispose: () => {
				const idx = listeners.indexOf(listener);
				if (idx !== -1) listeners.splice(idx, 1);
			},
		};
	};
	const fire = (arg: T) => {
		for (const l of listeners) l(arg);
	};
	return { event, fire };
}

export async function init(): Promise<void> {
	// No-op in tests – WASM is not available.
}

export class Terminal {
	cols = 80;
	rows = 24;
	element?: HTMLElement;
	textarea?: HTMLTextAreaElement;

	options: Record<string, unknown>;

	private _dataEvent = createEvent<string>();
	private _resizeEvent = createEvent<{ cols: number; rows: number }>();
	private _selectionEvent = createEvent<void>();
	private _customKeyHandler?: (event: KeyboardEvent) => boolean;
	private _content: HTMLDivElement | undefined;

	readonly buffer = {
		active: {
			getLine: (_y: number) => undefined as ReturnType<never> | undefined,
		},
	};

	readonly onData = this._dataEvent.event;
	readonly onResize = this._resizeEvent.event;
	readonly onSelectionChange = this._selectionEvent.event;

	constructor(opts: Record<string, unknown> = {}) {
		this.options = new Proxy(
			{ disableStdin: false, ...opts } as Record<string, unknown>,
			{
				set(target: Record<string, unknown>, prop: string, value: unknown) {
					target[prop] = value;
					return true;
				},
			},
		);
	}

	open(parent: HTMLElement) {
		this.element = parent;
		parent.classList.add("ghostty-terminal");

		this._content = document.createElement("div");
		this._content.classList.add("ghostty-rows");
		parent.appendChild(this._content);

		this.textarea = document.createElement("textarea");
		parent.appendChild(this.textarea);

		parent.addEventListener("keydown", (ev) => {
			if (this._customKeyHandler) {
				if (!this._customKeyHandler(ev)) return;
			}
		});
		parent.addEventListener("keyup", (ev) => {
			if (this._customKeyHandler) {
				if (!this._customKeyHandler(ev)) return;
			}
		});
	}

	write(data: string | Uint8Array) {
		if (!this._content) return;
		const text =
			typeof data === "string" ? data : new TextDecoder().decode(data);
		this._content.textContent = (this._content.textContent || "") + text;
	}

	writeln(data: string | Uint8Array) {
		if (typeof data === "string") {
			this.write(`${data}\r\n`);
		} else {
			this.write(data);
			this.write("\r\n");
		}
	}

	clear() {
		if (this._content) this._content.textContent = "";
	}

	focus() {
		this.element?.focus();
	}

	blur() {
		this.element?.blur();
	}

	resize(cols: number, rows: number) {
		this.cols = cols;
		this.rows = rows;
		this._resizeEvent.fire({ cols, rows });
	}

	getSelection(): string {
		return "";
	}

	hasSelection(): boolean {
		return false;
	}

	clearSelection() {}

	attachCustomKeyEventHandler(
		handler: (event: KeyboardEvent) => boolean,
	): void {
		this._customKeyHandler = handler;
	}

	registerLinkProvider(_provider: unknown): void {}

	loadAddon(addon: { activate(terminal: unknown): void }) {
		addon.activate(this);
	}

	dispose() {
		if (this._content?.parentElement) {
			this._content.parentElement.removeChild(this._content);
		}
	}

	// Allow tests to simulate user input.
	_fireData(data: string) {
		this._dataEvent.fire(data);
	}

	_fireResize(cols: number, rows: number) {
		this._resizeEvent.fire({ cols, rows });
	}
}

export class FitAddon {
	activate(_terminal: unknown) {}

	dispose() {}

	fit() {}

	proposeDimensions() {
		return { cols: 80, rows: 24 };
	}
}

// Re-export the ILinkProvider type as an empty interface for TS.
export type ILinkProvider = {
	provideLinks(
		y: number,
		callback: (links: unknown[] | undefined) => void,
	): void;
};
