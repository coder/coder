import {
	createContext,
	type FC,
	type PropsWithChildren,
	useContext,
	useRef,
	useState,
} from "react";
import {
	type AttachmentFailure,
	probeAttachmentFailure,
} from "../../utils/chatAttachments";

type ExpiredFileIdsContextValue = {
	getFailure: (fileId: string) => AttachmentFailure | undefined;
	hasExpired: (fileId: string) => boolean;
	markExpired: (fileId: string) => void;
	probeFailure: (fileId: string, href: string) => Promise<AttachmentFailure>;
};

const ExpiredFileIdsContext = createContext<ExpiredFileIdsContextValue>({
	getFailure: () => undefined,
	hasExpired: () => false,
	markExpired: () => {},
	probeFailure: async (_fileId, href) => probeAttachmentFailure(href),
});

export const ExpiredFileIdsProvider: FC<PropsWithChildren> = ({ children }) => {
	const [failures, setFailures] = useState<Map<string, AttachmentFailure>>(
		() => new Map(),
	);
	const failuresRef = useRef(failures);
	const inFlightProbes = useRef(new Map<string, Promise<AttachmentFailure>>());

	const rememberFailure = (fileId: string, failure: AttachmentFailure) => {
		const previousFailure = failuresRef.current.get(fileId);
		if (previousFailure?.kind === "expired" && failure.kind === "expired") {
			return;
		}
		if (
			previousFailure?.kind === "failed" &&
			failure.kind === "failed" &&
			previousFailure.detail === failure.detail
		) {
			return;
		}

		const next = new Map(failuresRef.current);
		next.set(fileId, failure);
		failuresRef.current = next;
		setFailures(next);
	};

	return (
		<ExpiredFileIdsContext.Provider
			value={{
				getFailure: (fileId) => failures.get(fileId),
				hasExpired: (fileId) => failures.get(fileId)?.kind === "expired",
				markExpired: (fileId) => rememberFailure(fileId, { kind: "expired" }),
				probeFailure: (fileId, href) => {
					const cachedFailure = failuresRef.current.get(fileId);
					if (cachedFailure) {
						return Promise.resolve(cachedFailure);
					}

					const inFlightProbe = inFlightProbes.current.get(fileId);
					if (inFlightProbe) {
						return inFlightProbe;
					}

					const probe = probeAttachmentFailure(href)
						.then((failure) => {
							rememberFailure(fileId, failure);
							return failure;
						})
						.finally(() => {
							inFlightProbes.current.delete(fileId);
						});
					inFlightProbes.current.set(fileId, probe);
					return probe;
				},
			}}
		>
			{children}
		</ExpiredFileIdsContext.Provider>
	);
};

export const useExpiredFileIds = () => useContext(ExpiredFileIdsContext);
