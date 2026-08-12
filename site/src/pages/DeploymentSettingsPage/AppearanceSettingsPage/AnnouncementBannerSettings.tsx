import { PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import type { BannerConfig } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Link } from "#/components/Link/Link";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { AnnouncementBannerDialog } from "./AnnouncementBannerDialog";
import { AnnouncementBannerItem } from "./AnnouncementBannerItem";

const DEFAULT_BANNER: BannerConfig = {
	enabled: true,
	message: "",
	background_color: "#ABB8C3",
};

type NewBannerButtonProps = {
	onClick: () => void;
};

const NewBannerButton: FC<NewBannerButtonProps> = ({ onClick }) => (
	<Button onClick={onClick} variant="outline">
		<PlusIcon />
		New announcement
	</Button>
);

interface AnnouncementBannersettingsProps {
	isEntitled: boolean;
	announcementBanners: readonly BannerConfig[];
	onSubmit: (banners: readonly BannerConfig[]) => Promise<void>;
}

type EditingBanner = {
	/** `null` means creating a new banner. */
	index: number | null;
	banner: BannerConfig;
};

export const AnnouncementBannerSettings: FC<
	AnnouncementBannersettingsProps
> = ({ isEntitled, announcementBanners, onSubmit }) => {
	const [banners, setBanners] = useState(announcementBanners);
	const [editingBanner, setEditingBanner] = useState<EditingBanner | null>(
		null,
	);
	const [deletingBannerId, setDeletingBannerId] = useState<number | null>(null);

	const openCreateDialog = () =>
		setEditingBanner({ index: null, banner: DEFAULT_BANNER });

	const updateBanner = (i: number, banner: Partial<BannerConfig>) => {
		const newBanners = [...banners];
		newBanners[i] = { ...banners[i], ...banner };
		setBanners(newBanners);
		return newBanners;
	};

	const removeBanner = (i: number) => {
		const newBanners = [...banners];
		newBanners.splice(i, 1);
		setBanners(newBanners);
		return newBanners;
	};

	const deletingBanner = deletingBannerId !== null && banners[deletingBannerId];

	return (
		<>
			<div>
				<SettingsHeader
					actions={
						isEntitled ? (
							<NewBannerButton onClick={openCreateDialog} />
						) : undefined
					}
				>
					<SettingsHeaderTitle hierarchy="secondary" level="h2">
						Announcement Banners
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Display message banners to all users.
						{!isEntitled && (
							<>
								{" "}
								Your license does not include Service Banners.{" "}
								<Link href="mailto:sales@coder.com" showExternalIcon={false}>
									Contact sales
								</Link>{" "}
								to learn more.
							</>
						)}
					</SettingsHeaderDescription>
				</SettingsHeader>

				<Table aria-label="Announcement banners">
					<TableHeader>
						<TableRow>
							<TableHead className="w-[1%] pl-5">Enabled</TableHead>
							<TableHead>Message</TableHead>
							<TableHead className="w-[2%]">Color</TableHead>
							<TableHead className="w-[1%]" />
						</TableRow>
					</TableHeader>
					<TableBody>
						{!isEntitled || banners.length < 1 ? (
							<TableEmpty
								message="No announcement banners"
								description="Create a banner to display a message to all users."
								cta={
									isEntitled ? (
										<NewBannerButton onClick={openCreateDialog} />
									) : undefined
								}
							/>
						) : (
							banners.map((banner, i) => (
								<AnnouncementBannerItem
									key={banner.message}
									enabled={banner.enabled && Boolean(banner.message)}
									backgroundColor={banner.background_color}
									message={banner.message}
									onEdit={() => setEditingBanner({ index: i, banner })}
									onUpdate={async (banner) => {
										const newBanners = updateBanner(i, banner);
										await onSubmit(newBanners);
									}}
									onDelete={() => setDeletingBannerId(i)}
								/>
							))
						)}
					</TableBody>
				</Table>
			</div>

			{editingBanner && (
				<AnnouncementBannerDialog
					banner={editingBanner.banner}
					onCancel={() => setEditingBanner(null)}
					onUpdate={async (banner) => {
						const nextBanner = { ...editingBanner.banner, ...banner };
						const newBanners =
							editingBanner.index === null
								? [...banners, nextBanner]
								: banners.map((existing, i) =>
										i === editingBanner.index ? nextBanner : existing,
									);
						setBanners(newBanners);
						setEditingBanner(null);
						await onSubmit(newBanners);
					}}
				/>
			)}

			{deletingBanner && (
				<ConfirmDialog
					type="delete"
					open
					title="Delete this banner?"
					description={deletingBanner.message}
					onClose={() => setDeletingBannerId(null)}
					onConfirm={async () => {
						const newBanners = removeBanner(deletingBannerId);
						setDeletingBannerId(null);
						await onSubmit(newBanners);
					}}
				/>
			)}
		</>
	);
};
