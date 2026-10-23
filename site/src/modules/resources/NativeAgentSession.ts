import {
	type InitializeResponse,
	methods,
	type SessionInfo,
	type SessionModeState,
	type SessionNotification,
	type ToolCallStatus,
} from "@agentclientprotocol/sdk";
import { getErrorMessage } from "#/api/errors";
import { WorkspaceAgentACP } from "#/api/workspaceAgentACP";
import { nativeAgentMetadata } from "./nativeAgentMetadata";

/** Text or tool status retained in the bounded transcript display. */
export type NativeAgentEntry =
	| {
			id: number;
			kind: "user" | "agent" | "thought";
			text: string;
			messageID?: string | null;
	  }
	| {
			id: number;
			kind: "tool";
			toolCallID: string;
			title: string;
			status: ToolCallStatus;
	  };

/** Retained display state for one native conversation, not durable history. */
export type NativeAgentSessionState = {
	info: SessionInfo;
	modes?: SessionModeState;
	entries: readonly NativeAgentEntry[];
	truncated: boolean;
	needsInput: boolean;
	error?: string;
	prompting: boolean;
};

/** Connection state and the selected conversation published to React. */
export type NativeAgentSnapshot = {
	status: "disconnected" | "connecting" | "connected";
	agentInfo?: InitializeResponse["agentInfo"];
	error?: string;
	session?: NativeAgentSessionState;
};

const MAX_ENTRIES = 100;
const MAX_TEXT = 8_000;
const MAX_SESSIONS = 10;

/** Bounded live ACP state. Notifications are published at most once per frame. */
export class NativeAgentSession {
	readonly transport: WorkspaceAgentACP;
	private readonly sessions = new Map<string, NativeAgentSessionState>();
	private readonly listeners = new Set<() => void>();
	private readonly pendingModes = new Map<string, string>();
	private activeID?: string;
	private creatingCwd?: string;
	private reloading = false;
	private connectionID = 0;
	private nextID = 0;
	private frame?: number;
	private snapshot: NativeAgentSnapshot = { status: "disconnected" };

	constructor(readonly agentID: string) {
		this.transport = new WorkspaceAgentACP(
			agentID,
			this.update,
			(sessionID) => {
				this.patch(sessionID, { needsInput: true });
			},
			(error) => {
				this.snapshot = {
					...this.snapshot,
					status: "disconnected",
					error: error.message,
				};
				this.publish();
			},
		);
	}

	getSnapshot = () => this.snapshot;
	subscribe = (listener: () => void) => {
		this.listeners.add(listener);
		return () => {
			this.listeners.delete(listener);
		};
	};

	private publish = () => {
		if (this.frame !== undefined) cancelAnimationFrame(this.frame);
		this.frame = undefined;
		this.snapshot = {
			...this.snapshot,
			session: this.reloading
				? this.snapshot.session
				: this.activeID
					? this.sessions.get(this.activeID)
					: undefined,
		};
		for (const listener of this.listeners) listener();
	};

	private patch(sessionID: string, patch: Partial<NativeAgentSessionState>) {
		const session = this.sessions.get(sessionID);
		if (!session) return;
		this.sessions.set(sessionID, { ...session, ...patch });
		if (this.frame === undefined)
			this.frame = requestAnimationFrame(this.publish);
	}

	private register(info: SessionInfo) {
		if (
			this.sessions.size >= MAX_SESSIONS &&
			!this.sessions.has(info.sessionId)
		) {
			throw new Error(
				"Reconnect before loading more sessions. Native sessions will keep running.",
			);
		}
		this.sessions.set(info.sessionId, {
			info,
			entries: [],
			truncated: false,
			needsInput: false,
			prompting: false,
		});
	}

	connect = async (signal?: AbortSignal) => {
		if (signal?.aborted) return;
		const connectionID = ++this.connectionID;
		const previous = this.snapshot.session;
		this.snapshot = {
			...this.snapshot,
			status: "connecting",
			error: undefined,
		};
		this.publish();
		try {
			const initialized = await this.transport.connect();
			if (connectionID !== this.connectionID) return;
			this.snapshot = { ...this.snapshot, agentInfo: initialized.agentInfo };
		} catch (error) {
			// An interrupted mount must not close its replacement connection.
			if (connectionID !== this.connectionID) return;
			this.close();
			this.snapshot = {
				...this.snapshot,
				error: getErrorMessage(error, String(error)),
			};
			this.publish();
			throw error;
		}
		// Replay into fresh state while keeping the prior transcript visible.
		this.reloading = Boolean(previous);
		this.sessions.clear();
		this.pendingModes.clear();
		try {
			if (previous) await this.load(previous.info);
		} catch (error) {
			if (connectionID !== this.connectionID) return;
			if (previous) {
				this.sessions.set(previous.info.sessionId, {
					...previous,
					prompting: false,
					error: getErrorMessage(error, "Session could not be reloaded."),
				});
			}
			throw error;
		} finally {
			if (connectionID === this.connectionID) {
				this.reloading = false;
				// A failed reload must not prevent listing or creating another session.
				this.snapshot = {
					...this.snapshot,
					status: this.transport.connected ? "connected" : "disconnected",
				};
				this.publish();
			}
		}
	};

	close = () => {
		this.connectionID++;
		this.transport.close();
		this.snapshot = { ...this.snapshot, status: "disconnected" };
		this.publish();
	};

	list = async (cursor?: string) => {
		const response = await this.transport.agent.request(
			methods.agent.session.list,
			{ cursor },
		);
		for (const info of response.sessions) this.applyInfo(info);
		this.publish();
		return response;
	};

	private applyInfo(info: SessionInfo) {
		const session = this.sessions.get(info.sessionId);
		if (!session) return;
		const { registrationPending } = nativeAgentMetadata(info);
		let modes = session.modes;
		if (
			modes &&
			registrationPending === false &&
			!modes.availableModes.some((item) => item.id === "automation")
		) {
			// Native registration enables this adapter's automation mode without
			// changing human control or reloading retained history.
			modes = {
				...modes,
				availableModes: [
					...modes.availableModes,
					{
						id: "automation",
						name: "Automation",
						description: "Submit ACP prompts.",
					},
				],
			};
		}
		this.patch(info.sessionId, {
			info,
			modes,
			needsInput:
				nativeAgentMetadata(session.info).registrationPending &&
				registrationPending === false
					? false
					: session.needsInput,
		});
	}

	create = async (cwd: string) => {
		if (!cwd.startsWith("/") || cwd.includes("\0"))
			throw new Error("Enter an absolute workspace directory.");
		if (this.sessions.size >= MAX_SESSIONS)
			throw new Error("Reconnect before creating more sessions.");
		if (this.creatingCwd)
			throw new Error("A session is already being created.");
		this.creatingCwd = cwd;
		try {
			const response = await this.transport.agent.request(
				methods.agent.session.new,
				{ cwd, mcpServers: [] },
			);
			const info: SessionInfo = {
				sessionId: response.sessionId,
				cwd,
				_meta: response._meta,
			};
			if (!this.sessions.has(info.sessionId)) this.register(info);
			this.activeID = info.sessionId;
			this.applyModes(info.sessionId, response.modes ?? undefined);
			this.applyInfo(info);
			this.publish();
			// Preserve the created identity even if switching mode fails. Native
			// onboarding must finish before requesting automation.
			if (!nativeAgentMetadata(info).registrationPending)
				await this.setMode("automation");
			return info;
		} finally {
			this.creatingCwd = undefined;
		}
	};

	load = async (info: SessionInfo) => {
		if (!this.sessions.has(info.sessionId)) {
			this.register(info);
			this.activeID = info.sessionId;
			this.publish();
			try {
				const response = await this.transport.agent.request(
					methods.agent.session.load,
					{
						sessionId: info.sessionId,
						cwd: info.cwd,
						mcpServers: [],
					},
				);
				const current = this.sessions.get(info.sessionId)?.info ?? info;
				this.applyModes(info.sessionId, response.modes ?? undefined);
				this.applyInfo({
					...current,
					_meta: { ...current._meta, ...response._meta },
				});
			} catch (error) {
				this.patch(info.sessionId, {
					error: getErrorMessage(error, "Session could not be loaded."),
				});
				this.publish();
				throw error;
			}
		}
		this.activeID = info.sessionId;
		this.publish();
		return info;
	};

	private applyModes(sessionID: string, modes: SessionModeState | undefined) {
		this.patch(sessionID, {
			modes: modes
				? {
						...modes,
						currentModeId:
							this.pendingModes.get(sessionID) ?? modes.currentModeId,
					}
				: undefined,
		});
		this.pendingModes.delete(sessionID);
	}

	setMode = async (modeId: string) => {
		const session = this.requireSession();
		if (session.error || nativeAgentMetadata(session.info).unavailable) {
			throw new Error(
				"This session is unavailable for control changes. Refresh its status before continuing.",
			);
		}
		if (
			modeId !== "human" &&
			nativeAgentMetadata(session.info).registrationPending
		) {
			throw new Error(
				"Join the native terminal to complete setup before selecting automation.",
			);
		}
		if (!session.modes?.availableModes.some((mode) => mode.id === modeId)) {
			throw new Error(`The adapter does not support ${modeId} mode.`);
		}
		await this.transport.agent.request(methods.agent.session.setMode, {
			sessionId: session.info.sessionId,
			modeId,
		});
		const current = this.sessions.get(session.info.sessionId);
		if (current?.modes && current.modes === session.modes)
			this.patch(session.info.sessionId, {
				modes: { ...current.modes, currentModeId: modeId },
			});
		this.publish();
	};

	prompt = async (text: string) => {
		const session = this.requireSession();
		if (
			session.modes?.currentModeId !== "automation" ||
			session.prompting ||
			session.error ||
			nativeAgentMetadata(session.info).unavailable ||
			nativeAgentMetadata(session.info).registrationPending
		) {
			throw new Error(
				"Select automation mode in a healthy session before sending a prompt.",
			);
		}
		this.patch(session.info.sessionId, {
			prompting: true,
			needsInput: false,
			truncated: session.truncated || text.length > MAX_TEXT,
		});
		this.append(session.info.sessionId, {
			id: this.nextID++,
			kind: "user",
			text: text.slice(-MAX_TEXT),
		});
		this.publish();
		try {
			return await this.transport.agent.request(methods.agent.session.prompt, {
				sessionId: session.info.sessionId,
				prompt: [{ type: "text", text }],
			});
		} finally {
			this.patch(session.info.sessionId, { prompting: false });
			this.publish();
		}
	};

	cancel = () =>
		this.transport.agent.notify(methods.agent.session.cancel, {
			sessionId: this.requireSession().info.sessionId,
		});

	private requireSession() {
		const session = this.activeID
			? this.sessions.get(this.activeID)
			: undefined;
		if (!session) throw new Error("Select a native session first.");
		return session;
	}

	private append(sessionID: string, entry: NativeAgentEntry) {
		const session = this.sessions.get(sessionID);
		if (!session) return;
		this.patch(sessionID, {
			entries: [...session.entries, entry].slice(-MAX_ENTRIES),
			truncated: session.truncated || session.entries.length >= MAX_ENTRIES,
		});
	}

	private update = ({ sessionId, update }: SessionNotification) => {
		// Agents may stream initialization messages before session/new responds.
		if (
			!this.sessions.has(sessionId) &&
			this.creatingCwd &&
			this.sessions.size < MAX_SESSIONS
		) {
			this.register({ sessionId, cwd: this.creatingCwd });
		}
		const session = this.sessions.get(sessionId);
		if (!session) return;
		switch (update.sessionUpdate) {
			case "user_message_chunk":
			case "agent_message_chunk":
			case "agent_thought_chunk": {
				if (update.content.type !== "text") break;
				const kind =
					update.sessionUpdate === "user_message_chunk"
						? "user"
						: update.sessionUpdate === "agent_thought_chunk"
							? "thought"
							: "agent";
				const last = session.entries.at(-1);
				const text = update.content.text;
				if (last?.kind === kind && last.messageID === update.messageId) {
					this.patch(sessionId, {
						entries: [
							...session.entries.slice(0, -1),
							{ ...last, text: (last.text + text).slice(-MAX_TEXT) },
						],
						truncated:
							session.truncated || last.text.length + text.length > MAX_TEXT,
					});
				} else {
					this.append(sessionId, {
						id: this.nextID++,
						kind,
						text: text.slice(-MAX_TEXT),
						messageID: update.messageId,
					});
					if (text.length > MAX_TEXT)
						this.patch(sessionId, { truncated: true });
				}
				this.patch(sessionId, { needsInput: false });
				break;
			}
			case "tool_call":
				this.append(sessionId, {
					id: this.nextID++,
					kind: "tool",
					toolCallID: update.toolCallId,
					title: update.title.slice(0, MAX_TEXT),
					status: update.status ?? "pending",
				});
				break;
			case "tool_call_update":
				this.patch(sessionId, {
					entries: session.entries.map((entry) =>
						entry.kind === "tool" && entry.toolCallID === update.toolCallId
							? {
									...entry,
									title: update.title?.slice(0, MAX_TEXT) ?? entry.title,
									status: update.status ?? entry.status,
								}
							: entry,
					),
				});
				break;
			case "current_mode_update":
				if (session.modes) {
					this.patch(sessionId, {
						modes: { ...session.modes, currentModeId: update.currentModeId },
					});
				} else {
					this.pendingModes.set(sessionId, update.currentModeId);
				}
				break;
			case "session_info_update": {
				const info = {
					...session.info,
					title: update.title || session.info.title,
				};
				const meta = update._meta?.["claude-acp-bridge"];
				if (meta && typeof meta === "object" && !Array.isArray(meta)) {
					const currentMeta = info._meta?.["claude-acp-bridge"];
					info._meta = {
						...info._meta,
						"claude-acp-bridge": {
							...(currentMeta && typeof currentMeta === "object"
								? currentMeta
								: {}),
							...meta,
						},
					};
				}
				this.applyInfo(info);
				if (!meta || typeof meta !== "object" || Array.isArray(meta)) break;
				if ("pendingInput" in meta && meta.pendingInput)
					this.patch(sessionId, { needsInput: true });
				if ("error" in meta && typeof meta.error === "string")
					this.patch(sessionId, { error: meta.error.slice(0, MAX_TEXT) });
				break;
			}
		}
	};
}
