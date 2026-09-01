import {
	Box,
	Button,
	Code,
	Drawer,
	DrawerBody,
	DrawerCloseButton,
	DrawerContent,
	DrawerOverlay,
	Flex,
	Grid,
	type GridProps,
	Heading,
	Icon,
	Img,
	Link,
	OrderedList,
	Table,
	TableContainer,
	Td,
	Text,
	Th,
	Thead,
	Tr,
	UnorderedList,
	useDisclosure,
} from "@chakra-ui/react";
import kebabCase from "lodash/kebabCase";
import type { ReactNode } from "react";
import { MdMenu } from "react-icons/md";
import ReactMarkdown from "react-markdown";
import { Link as RouterLink, useLocation } from "react-router";
import rehypeRaw from "rehype-raw";
import remarkGfm from "remark-gfm";
import {
	type DocsPageData,
	hrefForPath,
	type Nav,
	type NavItem,
	transformLinkUriSource,
} from "./content";

const asPathFromLocation = (pathname: string) => {
	if (pathname === "/") {
		return "/";
	}
	return pathname.endsWith("/") ? pathname : `${pathname}/`;
};

const SidebarNavItem = ({ item, nav }: { item: NavItem; nav: Nav }) => {
	const { pathname } = useLocation();
	const asPath = asPathFromLocation(pathname);
	let isActive = asPath.startsWith(`/${item.path}`);

	if (item.path === "") {
		isActive = asPath === "/";

		const homeNav = nav.find((navItem) => navItem.path === "") as NavItem;
		const homeNavPaths =
			homeNav.children?.map((child) => `/${child.path}/`) ?? [];
		if (homeNavPaths.includes(asPath)) {
			isActive = true;
		}
	}

	return (
		<Box>
			<Link
				as={RouterLink}
				to={hrefForPath(item.path)}
				fontWeight={isActive ? 600 : 400}
				color={isActive ? "gray.900" : "gray.700"}
			>
				{item.title}
			</Link>

			{isActive && item.children && (
				<Grid
					as="nav"
					pt={2}
					pl={3}
					maxW="sm"
					autoFlow="row"
					gap={2}
					autoRows="min-content"
				>
					{item.children.map((subItem) => (
						<SidebarNavItem key={subItem.path} item={subItem} nav={nav} />
					))}
				</Grid>
			)}
		</Box>
	);
};

const SidebarNav = ({ nav, ...gridProps }: { nav: Nav } & GridProps) => {
	return (
		<Grid
			h="100vh"
			overflowY="scroll"
			as="nav"
			p={8}
			w="300px"
			autoFlow="row"
			gap={2}
			autoRows="min-content"
			bgColor="white"
			borderRightWidth={1}
			borderColor="gray.200"
			borderStyle="solid"
			{...gridProps}
		>
			<Box mb={6}>
				<Img src="/logo.svg" alt="Coder logo" />
			</Box>

			{nav.map((navItem) => (
				<SidebarNavItem key={navItem.path} item={navItem} nav={nav} />
			))}
		</Grid>
	);
};

const MobileNavbar = ({ nav }: { nav: Nav }) => {
	const { isOpen, onOpen, onClose } = useDisclosure();

	return (
		<>
			<Flex
				bgColor="white"
				px={6}
				alignItems="center"
				h={16}
				borderBottomWidth={1}
			>
				<Img src="/logo.svg" alt="Coder logo" w={28} />

				<Button variant="ghost" ml="auto" onClick={onOpen}>
					<Icon as={MdMenu} fontSize="2xl" />
				</Button>
			</Flex>

			<Drawer onClose={onClose} isOpen={isOpen}>
				<DrawerOverlay />
				<DrawerContent>
					<DrawerCloseButton />
					<DrawerBody p={0}>
						<SidebarNav nav={nav} border={0} />
					</DrawerBody>
				</DrawerContent>
			</Drawer>
		</>
	);
};

const slugifyTitle = (titleSource: ReactNode) => {
	if (Array.isArray(titleSource) && typeof titleSource[0] === "string") {
		return kebabCase(titleSource[0].toLowerCase());
	}

	return undefined;
};

const getImageUrl = (src: string | undefined) => {
	if (src === undefined) {
		return "";
	}
	const assetPath = src.split("images/")[1];
	return `/images/${assetPath}`;
};

export const DocsPage = ({
	page,
	navigation,
}: {
	page: DocsPageData;
	navigation: Nav;
}) => {
	const { content, route, title } = page;

	return (
		<Box
			display={{ md: "grid" }}
			gridTemplateColumns="max-content 1fr"
			fontSize="md"
			color="gray.700"
		>
			<Box display={{ base: "none", md: "block" }}>
				<SidebarNav nav={navigation} />
			</Box>

			<Box display={{ base: "block", md: "none" }}>
				<MobileNavbar nav={navigation} />
			</Box>

			<Box
				as="main"
				w="full"
				pb={20}
				px={{ base: 6, md: 10 }}
				pl={{ base: 6, md: 20 }}
				h="100vh"
				overflowY="auto"
			>
				<Box maxW="872">
					<Box lineHeight="tall">
						<Heading
							as="h1"
							fontSize="4xl"
							pt={10}
							pb={2}
							sx={{ "& + h1": { display: "none" } }}
						>
							{title}
						</Heading>

						<ReactMarkdown
							rehypePlugins={[rehypeRaw]}
							remarkPlugins={[remarkGfm]}
							urlTransform={transformLinkUriSource(route.path)}
							components={{
								h1: ({ children }) => (
									<Heading
										as="h1"
										fontSize="4xl"
										pt={10}
										pb={2}
										id={slugifyTitle(children)}
									>
										{children}
									</Heading>
								),

								h2: ({ children }) => (
									<Heading
										as="h2"
										fontSize="3xl"
										pt={10}
										pb={2}
										id={slugifyTitle(children)}
									>
										{children}
									</Heading>
								),
								h3: ({ children }) => (
									<Heading
										as="h3"
										fontSize="2xl"
										pt={10}
										pb={2}
										id={slugifyTitle(children)}
									>
										{children}
									</Heading>
								),
								img: ({ src }) => (
									<Img
										src={getImageUrl(src)}
										mb={2}
										borderWidth={1}
										borderColor="gray.200"
										borderStyle="solid"
										rounded="md"
										height="auto"
									/>
								),
								p: ({ children }) => (
									<Text pt={2} pb={2}>
										{children}
									</Text>
								),
								ul: ({ children }) => (
									<UnorderedList
										mb={4}
										display="grid"
										gridAutoFlow="row"
										gap={2}
									>
										{children}
									</UnorderedList>
								),
								ol: ({ children }) => (
									<OrderedList mb={4} display="grid" gridAutoFlow="row" gap={2}>
										{children}
									</OrderedList>
								),
								a: ({ children, href = "" }) => {
									const isExternal =
										href.startsWith("http") || href.startsWith("https");

									return (
										<Link
											href={href}
											target={isExternal ? "_blank" : undefined}
											fontWeight={500}
											color="blue.600"
										>
											{children}
										</Link>
									);
								},
								code: ({ node, ...props }) => (
									<Code {...props} bgColor="gray.100" />
								),
								pre: ({ children }) => (
									<Box
										as="pre"
										w="full"
										sx={{ "& > code": { w: "full", p: 4, rounded: "md" } }}
										mb={2}
									>
										{children}
									</Box>
								),
								table: ({ children }) => (
									<TableContainer
										mt={1}
										mb={2}
										bgColor="white"
										rounded="md"
										borderWidth={1}
										borderColor="gray.100"
										borderStyle="solid"
									>
										<Table variant="simple">{children}</Table>
									</TableContainer>
								),
								thead: ({ children }) => <Thead>{children}</Thead>,
								th: ({ children }) => <Th>{children}</Th>,
								td: ({ children }) => <Td>{children}</Td>,
								tr: ({ children }) => <Tr>{children}</Tr>,
							}}
						>
							{content}
						</ReactMarkdown>
					</Box>
				</Box>
			</Box>
		</Box>
	);
};
