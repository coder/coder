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

type AttachmentFailureContextValue = {
	getFailure: (fileId: string) => AttachmentFailure | undefined;
	markExpired: (fileId: string) => void;
	probeFailure: (fileId: string, href: string) => Promise<AttachmentFailure>;
};

type CachedAttachmentFailure = Extract<AttachmentFailure, { kind: "expired" }>;

const AttachmentFailureContext = createContext<AttachmentFailureContextValue>({
	getFailure: () => undefined,
	markExpired: () => {},
	probeFailure: async (_fileId, href) => probeAttachmentFailure(href),
});

export const AttachmentFailureProvider: FC<PropsWithChildren> = ({
	children,
}) => {
	const [failures, setFailures] = useState<
		Map<string, CachedAttachmentFailure>
	>(() => new Map());
	const failuresRef = useRef(failures);
	const inFlightProbes = useRef(new Map<string, Promise<AttachmentFailure>>());

	const markExpired = (fileId: string) => {
		if (failuresRef.current.get(fileId)?.kind === "expired") {
			return;
		}

		const next = new Map(failuresRef.current);
		next.set(fileId, { kind: "expired" });
		failuresRef.current = next;
		setFailures(next);
	};

	return (
		<AttachmentFailureContext.Provider
			value={{
				getFailure: (fileId) => failures.get(fileId),
				markExpired,
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
							if (failure.kind === "expired") {
								markExpired(fileId);
							}
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
		</AttachmentFailureContext.Provider>
	);
};

export const useAttachmentFailure = () => useContext(AttachmentFailureContext);
