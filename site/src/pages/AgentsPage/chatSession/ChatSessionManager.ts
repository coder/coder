import { ChatSession } from "./ChatSession";
import type { ChatSessionManagerRuntimeDeps } from "./types";

const activeRetainedLimit = 5;
const warmInactiveLimit = 10;
const backgroundRetentionDelayMs = 500;

export type ChatSessionFactory = (
	chatId: string,
	deps: ChatSessionManagerRuntimeDeps,
) => ChatSession;

export class ChatSessionManager {
	private deps: ChatSessionManagerRuntimeDeps;

	private readonly sessionFactory: ChatSessionFactory;

	private readonly sessions = new Map<string, ChatSession>();

	private readonly activeRetainedLru = new Map<string, number>();

	private readonly warmInactiveLru = new Map<string, number>();

	private readonly retentionTimers = new Map<
		string,
		ReturnType<typeof setTimeout>
	>();

	public constructor(
		deps: ChatSessionManagerRuntimeDeps,
		sessionFactory: ChatSessionFactory = (chatId, deps) =>
			new ChatSession(chatId, deps),
	) {
		this.deps = deps;
		this.sessionFactory = sessionFactory;
	}

	public getOrCreate(chatId: string): ChatSession {
		const existing = this.sessions.get(chatId);
		if (existing) {
			return existing;
		}

		const session = this.sessionFactory(chatId, this.deps);
		this.sessions.set(chatId, session);
		return session;
	}

	public updateRuntimeDeps(nextDeps: ChatSessionManagerRuntimeDeps): void {
		this.deps = nextDeps;
		for (const session of this.sessions.values()) {
			session.updateRuntimeDeps(nextDeps);
		}
	}

	public markVisible(chatId: string): void {
		const session = this.getOrCreate(chatId);
		const now = Date.now();

		this.cancelRetentionTimer(chatId, session);
		this.removeFromLrus(chatId);
		session.enterForeground({ now });
	}

	public releaseVisible(chatId: string): void {
		const session = this.getOrCreate(chatId);
		const now = Date.now();

		this.cancelRetentionTimer(chatId, session);

		if (!this.isBackgroundRetentionEligible(session)) {
			session.disconnect();
			this.addWarmInactive(chatId, now);
			return;
		}

		const handle = setTimeout(() => {
			this.retentionTimers.delete(chatId);
			session.setRetentionTimer(null);

			if (this.sessions.get(chatId) !== session) {
				return;
			}
			if (!this.isBackgroundRetentionEligible(session)) {
				return;
			}

			const backgroundedAt = Date.now();
			session.enterBackgroundNoRead({ now: backgroundedAt });
			this.addActiveRetained(chatId, backgroundedAt);
		}, backgroundRetentionDelayMs);

		this.retentionTimers.set(chatId, handle);
		session.setRetentionTimer(handle);
	}

	public dispose(): void {
		for (const [chatId, handle] of this.retentionTimers) {
			clearTimeout(handle);
			const session = this.sessions.get(chatId);
			session?.setRetentionTimer(null);
		}
		this.retentionTimers.clear();

		for (const session of this.sessions.values()) {
			session.dispose();
		}
		this.sessions.clear();
		this.activeRetainedLru.clear();
		this.warmInactiveLru.clear();
	}

	private cancelRetentionTimer(chatId: string, session?: ChatSession): void {
		const handle = this.retentionTimers.get(chatId);
		if (handle) {
			clearTimeout(handle);
			this.retentionTimers.delete(chatId);
		}
		session?.setRetentionTimer(null);
	}

	private touchLru(
		lru: Map<string, number>,
		chatId: string,
		now: number,
	): void {
		lru.delete(chatId);
		lru.set(chatId, now);
	}

	private removeFromLrus(chatId: string): void {
		this.activeRetainedLru.delete(chatId);
		this.warmInactiveLru.delete(chatId);
	}

	private addActiveRetained(chatId: string, now: number): void {
		this.warmInactiveLru.delete(chatId);
		this.touchLru(this.activeRetainedLru, chatId, now);
		this.enforceActiveCap();
	}

	private addWarmInactive(chatId: string, now: number): void {
		this.activeRetainedLru.delete(chatId);
		this.touchLru(this.warmInactiveLru, chatId, now);
		this.enforceWarmCap();
	}

	private enforceActiveCap(): void {
		while (this.activeRetainedLru.size > activeRetainedLimit) {
			const oldestChatId = this.activeRetainedLru.keys().next().value;
			if (!oldestChatId) {
				return;
			}

			this.activeRetainedLru.delete(oldestChatId);
			const session = this.sessions.get(oldestChatId);
			if (!session) {
				continue;
			}

			session.disconnect();
			if (this.warmInactiveLru.size < warmInactiveLimit) {
				this.addWarmInactive(oldestChatId, Date.now());
				continue;
			}

			this.disposeSession(oldestChatId);
		}
	}

	private enforceWarmCap(): void {
		while (this.warmInactiveLru.size > warmInactiveLimit) {
			const oldestChatId = this.warmInactiveLru.keys().next().value;
			if (!oldestChatId) {
				return;
			}
			this.disposeSession(oldestChatId);
		}
	}

	private disposeSession(chatId: string): void {
		const session = this.sessions.get(chatId);
		if (!session) {
			this.removeFromLrus(chatId);
			return;
		}

		this.cancelRetentionTimer(chatId, session);
		session.dispose();
		this.sessions.delete(chatId);
		this.removeFromLrus(chatId);
	}

	private isBackgroundRetentionEligible(session: ChatSession): boolean {
		const metadata = session.getSnapshot();
		const { chatStatus } = session.store.getSnapshot();
		return (
			metadata.followMode &&
			(chatStatus === "running" || chatStatus === "pending")
		);
	}
}
