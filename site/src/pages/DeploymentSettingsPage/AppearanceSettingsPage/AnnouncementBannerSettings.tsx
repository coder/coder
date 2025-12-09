import { type CSSObject, useTheme } from "@emotion/react";
import Link from "@mui/material/Link";
import type { BannerConfig } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Stack } from "components/Stack/Stack";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { AnnouncementBannerDialog } from "./AnnouncementBannerDialog";
import { AnnouncementBannerItem } from "./AnnouncementBannerItem";

interface AnnouncementBannersettingsProps {
	isEntitled: boolean;
	announcementBanners: readonly BannerConfig[];
	onSubmit: (banners: readonly BannerConfig[]) => Promise<void>;
}

export const AnnouncementBannerSettings: FC<
	AnnouncementBannersettingsProps
> = ({ isEntitled, announcementBanners, onSubmit }) => {
	const theme = useTheme();
	const [banners, setBanners] = useState(announcementBanners);
	const [editingBannerId, setEditingBannerId] = useState<number | null>(null);
	const [deletingBannerId, setDeletingBannerId] = useState<number | null>(null);

	const addBanner = () => {
		setBanners([
			...banners,
			{ enabled: true, message: "", background_color: "#ABB8C3" },
		]);
		setEditingBannerId(banners.length);
	};

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

	const editingBanner = editingBannerId !== null && banners[editingBannerId];
	const deletingBanner = deletingBannerId !== null && banners[deletingBannerId];

	// If we're not editing a new banner, remove all empty banners. This makes canceling the
	// "new" dialog more intuitive, by not persisting an empty banner.
	if (editingBannerId === null && banners.some((banner) => !banner.message)) {
		setBanners(banners.filter((banner) => banner.message));
	}

	return (
		<>
			<div
				css={{
					border: `1px solid ${theme.palette.divider}`,
				}}
				className="rounded-lg mt-8 overflow-hidden"
			>
				<div className="p-6">
					<Stack
						direction="row"
						justifyContent="space-between"
						alignItems="center"
					>
						<h3 className="text-xl font-semibold m-0 leading-none">
							Announcement Banners
						</h3>
						<Button
							disabled={!isEntitled}
							onClick={() => addBanner()}
							variant="outline"
						>
							<PlusIcon />
							New
						</Button>
					</Stack>
					<div
						css={{
							color: theme.palette.text.secondary,
						}}
						className="text-sm leading-none"
					>
						Display message banners to all users.
					</div>

					<div css={[theme.typography.body2 as CSSObject]} className="pt-4">
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead className="w-[1%]">Enabled</TableHead>
									<TableHead>Message</TableHead>
									<TableHead className="w-[2%]">Color</TableHead>
									<TableHead className="w-[1%]" />
								</TableRow>
							</TableHeader>
							<TableBody>
								{!isEntitled || banners.length < 1 ? (
									<TableCell colSpan={999}>
										<EmptyState
											className="min-h-[160px]"
											message="No announcement banners"
										/>
									</TableCell>
								) : (
									banners.map((banner, i) => (
										<AnnouncementBannerItem
											key={banner.message}
											enabled={banner.enabled && Boolean(banner.message)}
											backgroundColor={banner.background_color}
											message={banner.message}
											onEdit={() => setEditingBannerId(i)}
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
				</div>

				{!isEntitled && (
					<footer
						css={[
							theme.typography.body2 as CSSObject,
							{
								background: theme.palette.background.paper,
							},
						]}
						className="py-4 px-6"
					>
						<div className="text-content-secondary">
							<p>
								Your license does not include Service Banners.{" "}
								<Link href="mailto:sales@coder.com">Contact sales</Link> to
								learn more.
							</p>
						</div>
					</footer>
				)}
			</div>

			{editingBanner && (
				<AnnouncementBannerDialog
					banner={editingBanner}
					onCancel={() => setEditingBannerId(null)}
					onUpdate={async (banner) => {
						const newBanners = updateBanner(editingBannerId, banner);
						setEditingBannerId(null);
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
